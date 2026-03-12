#!/usr/bin/env python3
"""
Production hygiene helper: purge (delete) content across the platform (admin-only).

It is intentionally guarded:
- default is dry-run
- to apply you must pass --apply and --confirm DELETE_ALL_CONTENT

Requires:
- ADMIN_API_KEY: admin user's API key (Bearer token)
- reachable BASE_URL
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _post_json(base_url: str, path: str, admin_api_key: str, payload: dict) -> dict:
    base = str(base_url).strip().rstrip("/")
    url = f"{base}{path}"
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url=url, data=body, method="POST")
    req.add_header("Authorization", f"Bearer {admin_api_key}")
    req.add_header("Content-Type", "application/json; charset=utf-8")
    try:
        with urllib.request.urlopen(req, timeout=300) as resp:
            data = resp.read()
        return json.loads(data.decode("utf-8", errors="replace") or "{}")
    except urllib.error.HTTPError as e:
        try:
            raw = e.read()
        except Exception:
            raw = b""
        msg = raw.decode("utf-8", errors="replace") if raw else str(e)
        raise SystemExit(f"HTTP error: status={getattr(e, 'code', '?')} body={msg}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default=os.environ.get("AIHUB_BASE_URL", "http://192.168.1.154:8080"))
    ap.add_argument("--apply", action="store_true", help="Actually delete (default is dry-run).")
    ap.add_argument("--confirm", default="", help="Required when --apply. Must be DELETE_ALL_CONTENT.")
    ap.add_argument("--no-runs", action="store_true")
    ap.add_argument("--no-agents", action="store_true")
    ap.add_argument("--no-topics", action="store_true")
    ap.add_argument("--no-reseed", action="store_true")
    args = ap.parse_args()

    admin_api_key = _env_required("ADMIN_API_KEY")

    payload = {
        "dry_run": (not bool(args.apply)),
        "confirm": str(args.confirm or "").strip(),
        "purge_runs": (not bool(args.no_runs)),
        "purge_agents": (not bool(args.no_agents)),
        "purge_topics": (not bool(args.no_topics)),
        "reseed": (not bool(args.no_reseed)),
    }
    path = "/v1/admin/content:purge"
    j = _post_json(str(args.base_url), path, admin_api_key, payload)

    dry_run = bool(j.get("dry_run"))
    counts = dict(j.get("counts") or {})
    result = dict(j.get("result") or {})

    print(f"[purge-content] dry_run={dry_run} counts={counts}", file=sys.stderr)
    if result:
        print(f"[purge-content] result={result}", file=sys.stderr)

    if dry_run:
        plan = list(j.get("plan") or [])
        if plan:
            print("[purge-content] plan:", file=sys.stderr)
            for step in plan:
                print(f"  - {step}", file=sys.stderr)
        print("[purge-content] dry-run only. Re-run with --apply --confirm DELETE_ALL_CONTENT to delete.", file=sys.stderr)
        return 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

