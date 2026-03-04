#!/usr/bin/env python3
"""
Minimal SSH exec helper (password-based) for Windows environments where docker runs on a remote host.

Design goals:
- No secrets in repo: password is read from an env var.
- Stream stdout/stderr and propagate remote exit code.
- Avoid swallowing errors: non-zero exit -> non-zero exit locally.
"""

from __future__ import annotations

import argparse
import os
import shlex
import sys
import time
from dataclasses import dataclass

import paramiko


@dataclass(frozen=True)
class SSHTarget:
    host: str
    port: int
    user: str
    password: str


def _read_env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _connect(t: SSHTarget, timeout_s: int) -> paramiko.SSHClient:
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(
        hostname=t.host,
        port=t.port,
        username=t.user,
        password=t.password,
        allow_agent=False,
        look_for_keys=False,
        timeout=timeout_s,
        banner_timeout=timeout_s,
        auth_timeout=timeout_s,
    )
    return c


def _build_shell_cmd(cmd: str, cwd: str | None) -> str:
    # Use sh -c for portability. Avoid "-l" to prevent noisy profile output from polluting stdout.
    inner = cmd
    if cwd:
        inner = f"cd {shlex.quote(cwd)} && {cmd}"
    return f"sh -c {shlex.quote(inner)}"


def _stream_channel(channel: paramiko.Channel, prefix: str) -> None:
    # Read incrementally so the caller sees progress for long-running commands.
    # We keep it simple; this is good enough for docker builds and smoke scripts.
    while True:
        progressed = False
        if channel.recv_ready():
            progressed = True
            # Write bytes directly to avoid Windows console encoding crashes (e.g. GBK can't encode ✓).
            sys.stdout.buffer.write(channel.recv(4096))
            sys.stdout.buffer.flush()
        if channel.recv_stderr_ready():
            progressed = True
            sys.stderr.buffer.write(channel.recv_stderr(4096))
            sys.stderr.buffer.flush()

        if channel.exit_status_ready():
            # Drain remaining output.
            while channel.recv_ready():
                sys.stdout.buffer.write(channel.recv(4096))
            while channel.recv_stderr_ready():
                sys.stderr.buffer.write(channel.recv_stderr(4096))
            sys.stdout.buffer.flush()
            sys.stderr.buffer.flush()
            return

        if not progressed:
            time.sleep(0.05)


def run_remote(
    client: paramiko.SSHClient,
    cmd: str,
    *,
    cwd: str | None,
    timeout_s: int,
    pass_env: list[str],
    show_cmd: bool,
    stdin_text: str | None = None,
    stdin_note: str | None = None,
) -> int:
    final_cmd = _build_shell_cmd(cmd, cwd)
    env = {}
    for k in pass_env:
        if k not in os.environ:
            raise SystemExit(f"Missing local env var to pass through: {k}")
        env[k] = str(os.environ[k])

    if show_cmd:
        # Do not print env var values; only show which names are passed.
        env_note = f" (pass env: {', '.join(pass_env)})" if pass_env else ""
        sin_note = f" (stdin: {stdin_note})" if stdin_note else ""
        print(f"[ssh] {client.get_transport().getpeername()[0]}$ {cmd}{env_note}{sin_note}", file=sys.stderr)

    transport = client.get_transport()
    if not transport:
        raise SystemExit("SSH transport is not available.")

    chan = transport.open_session(timeout=timeout_s)
    chan.settimeout(timeout_s)
    # Allocate a PTY for nicer streaming output, but NEVER when sending secrets via stdin,
    # because TTY echo can leak secret values into stdout.
    if stdin_text is None:
        chan.get_pty()
    if env:
        # Some sshd configs disallow AcceptEnv; callers should prefer stdin for secrets.
        chan.update_environment(env)
    chan.exec_command(final_cmd)
    if stdin_text is not None:
        chan.sendall(stdin_text)
        try:
            chan.shutdown_write()
        except Exception:
            # Not fatal; remote may already have closed stdin.
            pass
    _stream_channel(chan, prefix="")
    return int(chan.recv_exit_status())


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", required=True)
    ap.add_argument("--port", type=int, default=22)
    ap.add_argument("--user", default="root")
    ap.add_argument("--password-env", default="AIHUB_SSH_PASSWORD")
    ap.add_argument("--cwd", default=None)
    ap.add_argument("--timeout", type=int, default=1800, help="Connect/command timeout seconds")
    ap.add_argument("--pass-env", action="append", default=[], help="Env var name to pass to remote command")
    ap.add_argument("--show-cmd", action="store_true", help="Print remote command (no env values)")
    ap.add_argument("cmd", nargs=argparse.REMAINDER)
    args = ap.parse_args()

    if not args.cmd:
        raise SystemExit("Missing cmd to execute. Example: -- ls -la")
    cmd = " ".join(args.cmd).strip()
    if cmd.startswith("-- "):
        cmd = cmd[3:]

    t = SSHTarget(
        host=str(args.host),
        port=int(args.port),
        user=str(args.user),
        password=_read_env_required(str(args.password_env)),
    )

    client = _connect(t, timeout_s=int(args.timeout))
    try:
        code = run_remote(
            client,
            cmd,
            cwd=str(args.cwd) if args.cwd else None,
            timeout_s=int(args.timeout),
            pass_env=list(args.pass_env or []),
            show_cmd=bool(args.show_cmd),
        )
        return code
    finally:
        client.close()


if __name__ == "__main__":
    raise SystemExit(main())
