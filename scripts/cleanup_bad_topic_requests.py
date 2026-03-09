#!/usr/bin/env python3
"""
Production hygiene helper: find/delete clearly-bad OSS topic requests (admin-only).

Default is dry-run. Use --apply to actually delete.

This tool requires ADMIN_API_KEY (admin user api key) and a reachable BASE_URL.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
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
        with urllib.request.urlopen(req, timeout=60) as resp:
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
    ap.add_argument("--topic-id", required=True)
    ap.add_argument("--since-hours", type=int, default=72)
    ap.add_argument("--max-scan", type=int, default=2000)
    ap.add_argument("--max-delete", type=int, default=200)
    ap.add_argument("--apply", action="store_true", help="Actually delete (default is dry-run).")
    args = ap.parse_args()

    admin_api_key = _env_required("ADMIN_API_KEY")
    topic_id = str(args.topic_id).strip()
    if not topic_id:
        raise SystemExit("Missing --topic-id")

    payload = {
        "dry_run": (not bool(args.apply)),
        "since_hours": int(args.since_hours),
        "max_scan": int(args.max_scan),
        "max_delete": int(args.max_delete),
    }
    path = f"/v1/admin/oss/topics/{urllib.parse.quote(topic_id)}/requests:cleanup"
    j = _post_json(str(args.base_url), path, admin_api_key, payload)

    dry_run = bool(j.get("dry_run"))
    scanned = int(j.get("scanned") or 0)
    matched = int(j.get("matched") or 0)
    deleted = int(j.get("deleted") or 0)
    warnings = list(j.get("warnings") or [])
    items = list(j.get("items") or [])

    print(
        f"[cleanup-requests] topic_id={topic_id} dry_run={dry_run} scanned={scanned} matched={matched} deleted={deleted}",
        file=sys.stderr,
    )
    for it in items[:20]:
        ok = str(it.get("object_key") or "").strip()
        reason = str(it.get("reason") or "").strip()
        prev = str(it.get("text_preview") or "").strip().replace("\n", " ")
        if len(prev) > 140:
            prev = prev[:140] + "..."
        print(f"[cleanup-requests] match reason={reason} key={ok} preview={prev}", file=sys.stderr)
    if len(items) > 20:
        print(f"[cleanup-requests] ... ({len(items) - 20} more matches returned)", file=sys.stderr)

    for w in warnings[:20]:
        print(f"[cleanup-requests] warning {w}", file=sys.stderr)
    if len(warnings) > 20:
        print(f"[cleanup-requests] ... ({len(warnings) - 20} more warnings)", file=sys.stderr)

    if dry_run:
        print("[cleanup-requests] dry-run only. Re-run with --apply to delete.", file=sys.stderr)
        return 0
    if warnings:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

