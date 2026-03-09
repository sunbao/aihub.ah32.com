#!/usr/bin/env python3
"""
Probe "topic_play" background issuance on the docker host (default: 192.168.1.154).

Why:
- On Windows, passing quoted SQL through multiple shells is fragile.
- This script runs remote SQL via docker-compose+psql and prints a compact status report.

Secrets:
- SSH password is read from a local env var (default: AIHUB_SSH_PASSWORD).
- Nothing is written to the remote host.
"""

from __future__ import annotations

import argparse
import os
import pathlib
import sys

import paramiko

# Ensure repo root is on sys.path even when executing this file directly from scripts/remote/.
_REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(_REPO_ROOT))

from scripts.remote.aihub_154_deploy_and_smoke import _detect_repo_dir  # type: ignore  # noqa: E402
from scripts.remote.ssh_exec import SSHTarget, _connect, run_remote_capture  # type: ignore  # noqa: E402


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _psql(client: paramiko.SSHClient, *, repo: str, timeout_s: int, sql: str) -> str:
    # Use -A (unaligned) + -t (tuples only) for stable parsing.
    cmd = f'docker-compose exec -T db psql -U postgres -d aihub -Atc "{sql}"'
    code, out, err = run_remote_capture(
        client,
        cmd,
        cwd=repo,
        timeout_s=timeout_s,
        pass_env=[],
        show_cmd=False,
        stdin_text=None,
        stdin_note=None,
        max_capture_bytes=1024 * 1024,
    )
    if code != 0:
        raise SystemExit(f"remote psql failed: exit={code}\n{err.strip()}")
    return (out or "").strip()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="192.168.1.154")
    ap.add_argument("--port", type=int, default=22)
    ap.add_argument("--user", default="root")
    ap.add_argument("--password-env", default="AIHUB_SSH_PASSWORD")
    ap.add_argument("--repo-dir", default="")
    ap.add_argument("--timeout", type=int, default=120)
    ap.add_argument("--limit", type=int, default=12)
    args = ap.parse_args()

    t = SSHTarget(
        host=str(args.host),
        port=int(args.port),
        user=str(args.user),
        password=_env_required(str(args.password_env)),
    )
    client = _connect(t, timeout_s=int(args.timeout))
    try:
        repo = str(args.repo_dir).strip() or _detect_repo_dir(client, timeout_s=int(args.timeout))
        limit = int(args.limit)
        if limit <= 0:
            limit = 12

        head = _psql(client, repo=repo, timeout_s=int(args.timeout), sql="select now();")
        print(f"[topicplay] host={args.host} repo={repo} now={head}", file=sys.stderr)

        tags = _psql(
            client,
            repo=repo,
            timeout_s=int(args.timeout),
            sql=(
                "select t.tag||':'||count(*)::int "
                "from agent_tags t join agents a on a.id=t.agent_id "
                "where a.status='enabled' and t.tag like 'lobster%' "
                "group by t.tag order by t.tag;"
            ),
        )
        if tags:
            for line in tags.splitlines()[:50]:
                print(f"[topicplay] tag {line}", file=sys.stderr)
        else:
            print("[topicplay] no enabled lobster tags found", file=sys.stderr)

        work = _psql(
            client,
            repo=repo,
            timeout_s=int(args.timeout),
            sql="select count(*)::int||','||coalesce(max(created_at)::text,'') from work_items where stage='topic_play';",
        )
        print(f"[topicplay] work_items(stage=topic_play) {work}", file=sys.stderr)

        offers = _psql(
            client,
            repo=repo,
            timeout_s=int(args.timeout),
            sql=(
                "select to_char(wi.created_at,'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')||','||a.public_ref||','||a.name||','||wi.kind||','||wi.status "
                "from work_item_offers o "
                "join work_items wi on wi.id=o.work_item_id "
                "join agents a on a.id=o.agent_id "
                "where wi.stage='topic_play' "
                "order by wi.created_at desc "
                f"limit {limit};"
            ),
        )
        if offers:
            for line in offers.splitlines():
                print(f"[topicplay] offer {line}", file=sys.stderr)
        else:
            print("[topicplay] no offers found yet", file=sys.stderr)

        return 0
    finally:
        try:
            client.close()
        except Exception:
            pass


if __name__ == "__main__":
    raise SystemExit(main())

