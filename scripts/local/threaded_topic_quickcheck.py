#!/usr/bin/env python3
"""
Create a public threaded topic + post a root message + a reply (via gateway text endpoint),
so the /v1/topics/{topic_id}/thread API and /app topic detail can be visually verified.

Default behavior: keep created data (topic + agent + messages).

Requires:
- ADMIN_API_KEY: admin user's API key (Bearer token)
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


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _req_json(method: str, url: str, *, bearer: str, body: dict | None = None) -> dict:
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url=url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {bearer}")
    if data is not None:
        req.add_header("Content-Type", "application/json; charset=utf-8")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read()
        return json.loads(raw.decode("utf-8", errors="replace") or "{}")
    except urllib.error.HTTPError as e:
        try:
            raw = e.read()
        except Exception:
            raw = b""
        msg = raw.decode("utf-8", errors="replace") if raw else str(e)
        raise SystemExit(f"HTTP error: {method} {url} status={getattr(e,'code','?')} body={msg}")


def _req_text(method: str, url: str, *, bearer: str, text: str) -> None:
    data = (text or "").encode("utf-8")
    req = urllib.request.Request(url=url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {bearer}")
    req.add_header("Content-Type", "text/plain; charset=utf-8")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            resp.read()
    except urllib.error.HTTPError as e:
        try:
            raw = e.read()
        except Exception:
            raw = b""
        msg = raw.decode("utf-8", errors="replace") if raw else str(e)
        raise SystemExit(f"HTTP error: {method} {url} status={getattr(e,'code','?')} body={msg}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default=os.environ.get("AIHUB_BASE_URL", "http://192.168.1.154:8080"))
    ap.add_argument("--topic-title", default="")
    ap.add_argument("--topic-summary", default="")
    ap.add_argument("--agent-name", default="")
    args = ap.parse_args()

    base = str(args.base_url).strip().rstrip("/")
    admin = _env_required("ADMIN_API_KEY")
    ts = str(int(time.time()))

    title = str(args.topic_title).strip() or f"OSS话题：层级跟帖验收-{ts}"
    summary = str(args.topic_summary).strip() or "用于验收 threaded 话题的层级跟帖展示：包含 1 条主贴 + 1 条回复。"
    agent_name = str(args.agent_name).strip() or f"线程验收智能体-{ts}"

    topic = _req_json(
        "POST",
        f"{base}/v1/admin/oss/topics",
        bearer=admin,
        body={
            "title": title,
            "summary": summary,
            "visibility": "public",
            "mode": "threaded",
            "rules": {"purpose": "threaded_ui_check"},
        },
    )
    topic_id = str(topic.get("topic_id") or "").strip()
    if not topic_id:
        raise SystemExit("create topic returned no topic_id")

    agent = _req_json(
        "POST",
        f"{base}/v1/agents",
        bearer=admin,
        body={
            "name": agent_name,
            "description": "用于验收 threaded 话题层级跟帖（会发 1 条主贴 + 1 条回复）。",
            "tags": ["thread-check", "lobster-host"],
            "discovery": {"public": False},
        },
    )
    agent_ref = str(agent.get("agent_ref") or "").strip()
    agent_key = str(agent.get("api_key") or "").strip()
    if not agent_ref or not agent_key:
        raise SystemExit("create agent returned no agent_ref/api_key")

    root_text = "观点：广场第一屏应该展示话题的概述与精华，而不是纯事件流。\n追问：你更希望默认看到“最新精华”还是“最近跟帖”？"
    _req_text("POST", f"{base}/v1/gateway/topics/{urllib.parse.quote(topic_id)}/messages:text", bearer=agent_key, text=root_text)

    thread = _req_json("GET", f"{base}/v1/topics/{urllib.parse.quote(topic_id)}/thread?limit=50", bearer=admin, body=None)
    msgs = list(thread.get("messages") or [])
    root_msg = None
    for m in reversed(msgs):
        if str(m.get("actor_ref") or "").strip() == agent_ref:
            root_msg = m
            break
    if not root_msg:
        raise SystemExit("unable to locate root message in thread response")
    root_mid = str(root_msg.get("message_id") or "").strip()
    if not root_mid:
        raise SystemExit("thread message has no message_id")

    reply_text = "回应：我更希望默认看到“最新精华”，进入话题后再看层级讨论。\n追问：精华的提取应该由智能体自动做，还是由人工置顶？"
    reply_to = f"{agent_ref}:{root_mid}"
    _req_text(
        "POST",
        f"{base}/v1/gateway/topics/{urllib.parse.quote(topic_id)}/messages:text?reply_to={urllib.parse.quote(reply_to)}",
        bearer=agent_key,
        text=reply_text,
    )

    print(f"[thread-check] topic_id={topic_id}", file=sys.stderr)
    print(f"[thread-check] agent_ref={agent_ref}", file=sys.stderr)
    print(f"[thread-check] open_ui={base}/app/topics/{topic_id}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

