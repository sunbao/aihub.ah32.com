#!/usr/bin/env python3
"""
Deploy + smoke on the dedicated docker host (default: 192.168.1.154).

What it does (remote):
- Detect repo directory (or use --repo-dir).
- Pull + rebuild + restart: bash scripts/update.sh
- Run smoke suites on the host: bash scripts/smoke.sh + smoke_moderation.sh

Notes:
- Secrets are read from local env vars; nothing is written to disk.
- The smoke scripts require: bash + curl + jq on the remote host.
"""

from __future__ import annotations

import argparse
import os
import pathlib
import sys
from datetime import datetime, timezone

import paramiko

# Ensure repo root is on sys.path even when executing this file directly from scripts/remote/.
_REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(_REPO_ROOT))

from scripts.remote.ssh_exec import SSHTarget, _connect, run_remote  # type: ignore  # noqa: E402


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _detect_repo_dir(client: paramiko.SSHClient, timeout_s: int) -> str:
    candidates = [
        "/root/AIHub",
        "/root/aihub",
        "/opt/AIHub",
        "/opt/aihub",
        "/srv/AIHub",
        "/srv/aihub",
        "/data/aihub.ah32.com",
    ]
    for d in candidates:
        code = run_remote(
            client,
            f'test -f "{d}/docker-compose.yml"',
            cwd=None,
            timeout_s=timeout_s,
            pass_env=[],
            show_cmd=False,
        )
        if code == 0:
            return d

    # Fallback: try to locate by name but limit scope to keep it safe.
    # If this fails, the user should provide --repo-dir explicitly.
    code = run_remote(
        client,
        r'for d in /root/* /opt/* /srv/* /data/*; do [ -f "$d/docker-compose.yml" ] && echo "$d" && exit 0; done; exit 2',
        cwd=None,
        timeout_s=timeout_s,
        pass_env=[],
        show_cmd=False,
    )
    if code != 0:
        raise SystemExit("Unable to auto-detect repo dir on 154. Provide --repo-dir (e.g. /root/AIHub).")

    # The echo from the remote command was already streamed to stdout; that's not stable to parse here.
    # We re-run with a command that prints exactly one path and we capture it.
    stdin, stdout, stderr = client.exec_command(
        'sh -c \'for d in /root/* /opt/* /srv/* /data/*; do [ -f "$d/docker-compose.yml" ] && echo "$d" && exit 0; done; exit 2\''
    )
    out = stdout.read().decode("utf-8", errors="replace").strip().splitlines()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    if err:
        print(err, file=sys.stderr)
    if not out:
        raise SystemExit("Unable to auto-detect repo dir on 154. Provide --repo-dir.")
    return out[0].strip()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="192.168.1.154")
    ap.add_argument("--port", type=int, default=22)
    ap.add_argument("--user", default="root")
    ap.add_argument("--password-env", default="AIHUB_SSH_PASSWORD")
    ap.add_argument("--repo-dir", default="")
    ap.add_argument("--timeout", type=int, default=3600)
    ap.add_argument("--base-url", default="http://127.0.0.1:8080")
    ap.add_argument("--skip-deploy", action="store_true")
    ap.add_argument("--skip-smoke", action="store_true")
    ap.add_argument("--show-cmd", action="store_true")
    args = ap.parse_args()

    # Required only when running smoke scripts.
    admin_api_key = ""
    if not args.skip_smoke:
        admin_api_key = _env_required("ADMIN_API_KEY")

    t = SSHTarget(
        host=str(args.host),
        port=int(args.port),
        user=str(args.user),
        password=_env_required(str(args.password_env)),
    )

    client = _connect(t, timeout_s=int(args.timeout))
    try:
        repo = str(args.repo_dir).strip() or _detect_repo_dir(client, timeout_s=int(args.timeout))
        ts = datetime.now(timezone.utc).isoformat(timespec="seconds")
        print(f"[aihub] target={args.host} repo={repo} utc={ts}", file=sys.stderr)

        # Preconditions
        if not args.skip_smoke:
            pre = run_remote(
                client,
                'command -v bash >/dev/null && command -v curl >/dev/null && command -v jq >/dev/null',
                cwd=repo,
                timeout_s=int(args.timeout),
                pass_env=[],
                show_cmd=bool(args.show_cmd),
            )
            if pre != 0:
                raise SystemExit("Remote host missing required tools for smoke: bash/curl/jq.")

        if not args.skip_deploy:
            code = run_remote(
                client,
                "bash scripts/update.sh",
                cwd=repo,
                timeout_s=int(args.timeout),
                pass_env=[],
                show_cmd=bool(args.show_cmd),
            )
            if code != 0:
                return code

        if not args.skip_smoke:
            smoke = run_remote(
                client,
                # Read secret from stdin to avoid SSH env forwarding issues and keep it out of argv/history.
                f'bash -c \'read -r ADMIN_API_KEY; export ADMIN_API_KEY; BASE="{args.base_url}" bash scripts/smoke.sh\'',
                cwd=repo,
                timeout_s=int(args.timeout),
                pass_env=[],
                show_cmd=bool(args.show_cmd),
                stdin_text=f"{admin_api_key}\n",
                stdin_note="ADMIN_API_KEY",
            )
            if smoke != 0:
                return smoke

            mod = run_remote(
                client,
                f'bash -c \'read -r ADMIN_API_KEY; export ADMIN_API_KEY; BASE="{args.base_url}" bash scripts/smoke_moderation.sh\'',
                cwd=repo,
                timeout_s=int(args.timeout),
                pass_env=[],
                show_cmd=bool(args.show_cmd),
                stdin_text=f"{admin_api_key}\n",
                stdin_note="ADMIN_API_KEY",
            )
            if mod != 0:
                return mod

        return 0
    finally:
        client.close()


if __name__ == "__main__":
    raise SystemExit(main())
