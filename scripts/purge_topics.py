#!/usr/bin/env python3
"""
Production hygiene helper: purge (delete) OSS-backed topics and their oss_events (admin-only).

Default is dry-run. Use --apply with --confirm DELETE_ALL_TOPICS to actually delete.

This tool requires ADMIN_API_KEY (admin user api key) and a reachable BASE_URL.
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
        with urllib.request.urlopen(req, timeout=120) as resp:
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
    ap.add_argument("--max-scan", type=int, default=5000)
    ap.add_argument("--max-delete", type=int, default=200)
    ap.add_argument("--keep-topic-id", action="append", default=[], help="Repeatable. Defaults to keeping topic_daily_checkin.")
    ap.add_argument("--apply", action="store_true", help="Actually delete (default is dry-run).")
    ap.add_argument("--confirm", default="", help="Required when --apply. Must be DELETE_ALL_TOPICS.")
    args = ap.parse_args()

    admin_api_key = _env_required("ADMIN_API_KEY")
    keep = [str(x).strip() for x in (args.keep_topic_id or []) if str(x).strip()]

    payload = {
        "dry_run": (not bool(args.apply)),
        "confirm": str(args.confirm or "").strip(),
        "keep_topic_ids": keep,
        "max_scan": int(args.max_scan),
        "max_delete": int(args.max_delete),
    }
    path = "/v1/admin/oss/topics:purge"
    j = _post_json(str(args.base_url), path, admin_api_key, payload)

    dry_run = bool(j.get("dry_run"))
    scanned = int(j.get("scanned_distinct_topics") or 0)
    matched = int(j.get("matched_topics") or 0)
    deleted = int(j.get("deleted_topics") or 0)
    keep_out = list(j.get("keep_topic_ids") or [])

    print(
        f"[purge-topics] dry_run={dry_run} scanned_distinct_topics={scanned} matched_topics={matched} deleted_topics={deleted}",
        file=sys.stderr,
    )
    if keep_out:
        print(f"[purge-topics] keep_topic_ids={keep_out}", file=sys.stderr)

    items = list(j.get("items") or [])
    for it in items[:30]:
        tid = str(it.get("topic_id") or "").strip()
        skipped = bool(it.get("skipped"))
        if skipped:
            sr = str(it.get("skip_reason") or "").strip()
            print(f"[purge-topics] skip topic_id={tid} reason={sr}", file=sys.stderr)
            continue
        osd = int(it.get("oss_deleted") or 0)
        dbd = int(it.get("db_deleted") or 0)
        print(f"[purge-topics] topic_id={tid} oss_deleted={osd} db_deleted={dbd}", file=sys.stderr)
    if len(items) > 30:
        print(f"[purge-topics] ... ({len(items) - 30} more items returned)", file=sys.stderr)

    if dry_run:
        print("[purge-topics] dry-run only. Re-run with --apply --confirm DELETE_ALL_TOPICS to delete.", file=sys.stderr)
        return 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

