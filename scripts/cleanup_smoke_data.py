#!/usr/bin/env python3
"""
Production hygiene helper: delete old smoke/e2e test data via HTTP APIs.

Default is dry-run. Use --apply to actually delete.

Targets:
- Runs: /v1/runs?q=Smoke... where goal starts with "Smoke:" or "Smoke moderation:".
- Agents: /v1/admin/agents?q=... where name starts with "smoke-agent-", "smoke-mod-agent-", or "e2e-".

This tool requires ADMIN_API_KEY (admin user api key) and a reachable BASE_URL.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _now_utc() -> datetime:
    return datetime.now(timezone.utc)


def _parse_rfc3339(s: str) -> datetime | None:
    v = str(s or "").strip()
    if not v:
        return None
    # Server uses time.RFC3339
    try:
        return datetime.fromisoformat(v.replace("Z", "+00:00")).astimezone(timezone.utc)
    except Exception:
        return None


@dataclass(frozen=True)
class Api:
    base_url: str
    admin_api_key: str

    def _req(self, method: str, path: str, *, query: dict[str, str] | None = None) -> urllib.request.Request:
        base = self.base_url.rstrip("/")
        url = f"{base}{path}"
        if query:
            qs = urllib.parse.urlencode(query)
            url = f"{url}?{qs}"
        req = urllib.request.Request(url=url, method=method)
        req.add_header("Authorization", f"Bearer {self.admin_api_key}")
        return req

    def get_json(self, path: str, *, query: dict[str, str] | None = None) -> dict:
        req = self._req("GET", path, query=query)
        with urllib.request.urlopen(req, timeout=20) as resp:
            body = resp.read()
        return json.loads(body.decode("utf-8", errors="replace") or "{}")

    def delete(self, path: str) -> int:
        req = self._req("DELETE", path)
        try:
            with urllib.request.urlopen(req, timeout=20) as resp:
                resp.read()
                return int(getattr(resp, "status", 200) or 200)
        except urllib.error.HTTPError as e:
            # HTTPError is also a file-like response.
            try:
                e.read()
            except Exception:
                pass
            return int(getattr(e, "code", 0) or 0)


def is_smoke_run_goal(goal: str) -> bool:
    g = str(goal or "").strip()
    return g.startswith("Smoke:") or g.startswith("Smoke moderation:")


def is_smoke_like_agent_name(name: str) -> bool:
    n = str(name or "").strip().lower()
    return n.startswith("smoke-agent-") or n.startswith("smoke-mod-agent-") or n.startswith("e2e-")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default=os.environ.get("AIHUB_BASE_URL", "http://192.168.1.154:8080"))
    ap.add_argument("--apply", action="store_true", help="Actually delete (default is dry-run).")
    ap.add_argument("--max-runs", type=int, default=200)
    ap.add_argument("--max-agents", type=int, default=200)
    ap.add_argument("--max-age-hours", type=int, default=24 * 14, help="Only delete items created within this age window.")
    args = ap.parse_args()

    admin_api_key = _env_required("ADMIN_API_KEY")
    api = Api(base_url=str(args.base_url).strip(), admin_api_key=admin_api_key)

    cutoff = _now_utc().timestamp() - float(args.max_age_hours) * 3600.0

    to_delete_runs: list[dict] = []
    offset = 0
    while len(to_delete_runs) < int(args.max_runs):
        j = api.get_json(
            "/v1/runs",
            query={
                "q": "Smoke",
                "include_system": "1",
                "limit": "50",
                "offset": str(offset),
            },
        )
        runs = list(j.get("runs") or [])
        if not runs:
            break
        for r in runs:
            run_ref = str(r.get("run_ref") or "").strip()
            goal = str(r.get("goal") or "").strip()
            created_at = _parse_rfc3339(str(r.get("created_at") or "").strip())
            if not run_ref or not is_smoke_run_goal(goal):
                continue
            if created_at and created_at.timestamp() < cutoff:
                continue
            to_delete_runs.append({"run_ref": run_ref, "goal": goal, "created_at": created_at.isoformat() if created_at else ""})
            if len(to_delete_runs) >= int(args.max_runs):
                break
        offset += len(runs)
        if offset > 5000:
            break

    agent_queries = ["smoke-agent-", "smoke-mod-agent-", "e2e-"]
    to_delete_agents: list[dict] = []
    for q in agent_queries:
        if len(to_delete_agents) >= int(args.max_agents):
            break
        offset = 0
        while len(to_delete_agents) < int(args.max_agents):
            j = api.get_json("/v1/admin/agents", query={"q": q, "limit": "100", "offset": str(offset)})
            items = list(j.get("items") or [])
            if not items:
                break
            for it in items:
                agent_ref = str(it.get("agent_ref") or "").strip()
                name = str(it.get("name") or "").strip()
                updated_at = _parse_rfc3339(str(it.get("updated_at") or "").strip())
                if not agent_ref or not is_smoke_like_agent_name(name):
                    continue
                if updated_at and updated_at.timestamp() < cutoff:
                    continue
                to_delete_agents.append({"agent_ref": agent_ref, "name": name, "updated_at": updated_at.isoformat() if updated_at else ""})
                if len(to_delete_agents) >= int(args.max_agents):
                    break
            has_more = bool(j.get("has_more"))
            if not has_more:
                break
            offset = int(j.get("next_offset") or (offset + len(items)))
            if offset > 50000:
                break

    def print_plan() -> None:
        print(f"[cleanup] base_url={api.base_url} dry_run={not args.apply} cutoff_hours={args.max_age_hours}", file=sys.stderr)
        print(f"[cleanup] runs_to_delete={len(to_delete_runs)} agents_to_delete={len(to_delete_agents)}", file=sys.stderr)
        for r in to_delete_runs[:10]:
            print(f"[cleanup] run {r['run_ref']} goal={r['goal']}", file=sys.stderr)
        if len(to_delete_runs) > 10:
            print(f"[cleanup] ... ({len(to_delete_runs) - 10} more runs)", file=sys.stderr)
        for a in to_delete_agents[:10]:
            print(f"[cleanup] agent {a['agent_ref']} name={a['name']}", file=sys.stderr)
        if len(to_delete_agents) > 10:
            print(f"[cleanup] ... ({len(to_delete_agents) - 10} more agents)", file=sys.stderr)

    print_plan()
    if not args.apply:
        return 0

    failed = 0

    # Delete runs first to remove dependent work items/events.
    for r in to_delete_runs:
        ref = str(r["run_ref"])
        st = api.delete(f"/v1/admin/runs/{urllib.parse.quote(ref)}")
        if st not in (200, 404):
            print(f"[cleanup] delete run failed run_ref={ref} status={st}", file=sys.stderr)
            failed += 1
        time.sleep(0.05)

    for a in to_delete_agents:
        ref = str(a["agent_ref"])
        st = api.delete(f"/v1/agents/{urllib.parse.quote(ref)}")
        if st not in (200, 404):
            print(f"[cleanup] delete agent failed agent_ref={ref} status={st}", file=sys.stderr)
            failed += 1
        time.sleep(0.05)

    if failed:
        print(f"[cleanup] done with failures={failed}", file=sys.stderr)
        return 1
    print("[cleanup] done ok", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

