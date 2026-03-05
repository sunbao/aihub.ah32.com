#!/usr/bin/env python3
"""
Seed a curated set of "role agents" (owned by the caller) via HTTP APIs.

Why:
- The product already supports agent cards (name/description/interests/capabilities/bio/greeting + optional persona template).
- This script creates a consistent set of agents with distinct names and intended functions.

Safety:
- Default is dry-run (prints planned changes).
- Idempotent: skips creation when an agent with the same name already exists for the caller.
- Can cleanup created agents by tag.

Requires:
- ADMIN_API_KEY: an is_admin=true user's API key (used as the caller's Bearer token)
  Note: despite the name, it's NOT a server env var; it's a user api key from browser localStorage.
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
from typing import Any


def _env_required(name: str) -> str:
    v = str(os.environ.get(name, "")).strip()
    if not v:
        raise SystemExit(f"Missing required env var: {name}")
    return v


def _die(msg: str) -> None:
    print(msg, file=sys.stderr)
    raise SystemExit(2)


def _now_ms() -> int:
    return int(time.time() * 1000)


@dataclass(frozen=True)
class API:
    base_url: str
    api_key: str
    timeout_s: int

    def _req(self, method: str, path: str, body: dict[str, Any] | None = None) -> urllib.request.Request:
        url = self.base_url.rstrip("/") + path
        data = None
        if body is not None:
            data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        r = urllib.request.Request(url=url, data=data, method=method)
        r.add_header("Authorization", f"Bearer {self.api_key}")
        if body is not None:
            r.add_header("Content-Type", "application/json; charset=utf-8")
        return r

    def get_json(self, path: str) -> dict[str, Any]:
        req = self._req("GET", path, None)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as resp:
                raw = resp.read().decode("utf-8", errors="replace")
                return json.loads(raw)
        except urllib.error.HTTPError as e:
            raw = (e.read() or b"").decode("utf-8", errors="replace")
            _die(f"GET {path} failed: status={e.code} body={raw}")
        except Exception as e:
            _die(f"GET {path} failed: {e}")
        raise AssertionError("unreachable")

    def post_json(self, path: str, body: dict[str, Any]) -> dict[str, Any]:
        req = self._req("POST", path, body)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as resp:
                raw = resp.read().decode("utf-8", errors="replace")
                return json.loads(raw)
        except urllib.error.HTTPError as e:
            raw = (e.read() or b"").decode("utf-8", errors="replace")
            _die(f"POST {path} failed: status={e.code} body={raw}")
        except Exception as e:
            _die(f"POST {path} failed: {e}")
        raise AssertionError("unreachable")

    def delete(self, path: str) -> int:
        req = self._req("DELETE", path, None)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as resp:
                return int(resp.status)
        except urllib.error.HTTPError as e:
            raw = (e.read() or b"").decode("utf-8", errors="replace")
            _die(f"DELETE {path} failed: status={e.code} body={raw}")
        except Exception as e:
            _die(f"DELETE {path} failed: {e}")
        raise AssertionError("unreachable")


def _role_presets(tag: str) -> list[dict[str, Any]]:
    # Use existing built-in persona templates where possible.
    # These presets encode "function" mainly via description/capabilities/bio/greeting.
    suffix = str(_now_ms())[-4:]
    return [
        {
            "name": f"回归测试官-{suffix}",
            "description": "负责端到端回归：复现问题、定位原因、给出最小修复建议与复测清单。",
            "tags": [tag, "role:qa", "scope:e2e"],
            "persona_template_id": "persona_rigorous_engineer_v1",
            "interests": ["质量", "稳定性", "可观测性"],
            "capabilities": ["端到端测试", "缺陷复现", "日志分析", "回归清单", "风险评估"],
            "bio": "我关注可验证的事实与复现步骤，优先给出最小可行的修复与复测路径。",
            "greeting": "把你要测的场景和预期发我，我先给出复现步骤和检查点。",
            "discovery": {"public": False},
        },
        {
            "name": f"测评策划官-{suffix}",
            "description": "负责测评/话题策划：选择话题、设置评测维度、定义通过标准和回归用例。",
            "tags": [tag, "role:evaluation", "scope:planning"],
            "persona_template_id": "persona_pragmatic_pm_v1",
            "interests": ["用户价值", "需求拆解", "验收标准"],
            "capabilities": ["话题选择", "评测维度设计", "验收标准", "用例设计", "前后端联调"],
            "bio": "我把目标、约束、验收标准写清楚，再把用例拆成可执行步骤。",
            "greeting": "告诉我你要评测的目标和边界，我给你一套可执行的测评方案。",
            "discovery": {"public": False},
        },
        {
            "name": f"注入向导-龙虾-{suffix}",
            "description": "负责一键注入相关：解释注入命令、前置条件、注入后验证点与回滚路径。",
            "tags": [tag, "role:openclaw", "scope:ops"],
            "persona_template_id": "persona_rigorous_engineer_v1",
            "interests": ["自动化", "可靠性", "可回滚"],
            "capabilities": ["注入命令指导", "前置检查", "结果验证", "回滚建议"],
            "bio": "我会把注入前置条件和注入后验证点写成清单，确保可控、可回滚。",
            "greeting": "把你要注入的目标和环境发我，我先做前置检查清单。",
            "discovery": {"public": False},
        },
        {
            "name": f"内容审核官-{suffix}",
            "description": "负责审核/风控：检查队列、给出可执行的驳回理由与修复建议。",
            "tags": [tag, "role:moderation", "scope:policy"],
            "persona_template_id": "persona_rigorous_engineer_v1",
            "interests": ["合规", "安全", "清晰沟通"],
            "capabilities": ["审核判断", "风险点归因", "驳回理由", "整改建议"],
            "bio": "我会按规则给出明确的驳回原因和可操作的整改建议，避免模糊表述。",
            "greeting": "把待审核内容和上下文发我，我按规则输出结论和整改项。",
            "discovery": {"public": False},
        },
        {
            "name": f"发布助手-{suffix}",
            "description": "负责发布流程：发布前检查、灰度/回滚策略、发布后验证与告警检查。",
            "tags": [tag, "role:release", "scope:ops"],
            "persona_template_id": "persona_pragmatic_pm_v1",
            "interests": ["上线质量", "回滚策略", "用户影响"],
            "capabilities": ["发布清单", "灰度策略", "回滚方案", "发布后验证"],
            "bio": "我用 checklist 把上线风险降到可控范围，优先明确回滚和验证点。",
            "greeting": "告诉我你要发布的变更点，我给发布清单和验证点。",
            "discovery": {"public": False},
        },
        {
            "name": f"周报分析师-{suffix}",
            "description": "负责周报/数据解读：把关键指标、异常点和下一步动作写清楚。",
            "tags": [tag, "role:analytics", "scope:report"],
            "persona_template_id": "persona_data_analyst_v1",
            "interests": ["指标", "趋势", "归因"],
            "capabilities": ["指标解读", "异常分析", "归因假设", "下一步行动"],
            "bio": "我把数据结论转成可执行的下一步动作，并标注不确定性与验证方法。",
            "greeting": "把周报或关键指标发我，我输出结论、风险点和行动项。",
            "discovery": {"public": False},
        },
    ]


def _list_agent_names(api: API) -> set[str]:
    j = api.get_json("/v1/agents")
    names: set[str] = set()
    for it in (j.get("agents") or []):
        try:
            n = str(it.get("name") or "").strip()
        except Exception:
            n = ""
        if n:
            names.add(n)
    return names


def _create_agents(api: API, presets: list[dict[str, Any]], apply: bool) -> int:
    existing = _list_agent_names(api)
    planned = 0
    created = 0
    for p in presets:
        name = str(p.get("name") or "").strip()
        if not name:
            print("[seed] skip empty name preset", file=sys.stderr)
            continue
        if name in existing:
            print(f"[seed] exists; skip name={name}", file=sys.stderr)
            continue
        planned += 1
        if not apply:
            print(f"[seed] dry-run create name={name}", file=sys.stderr)
            continue
        body = dict(p)
        body.pop("seed_only", None)
        out = api.post_json("/v1/agents", body)
        agent_ref = str(out.get("agent_ref") or "").strip()
        if not agent_ref:
            _die(f"create agent returned no agent_ref for name={name}")
        created += 1
        print(f"[seed] created name={name} agent_ref={agent_ref}", file=sys.stderr)
    print(f"[seed] planned={planned} created={created} apply={apply}", file=sys.stderr)
    return 0


def _cleanup_agents(api: API, tag: str, apply: bool) -> int:
    # There's no owner-side tag filter API today; list then delete by tag match.
    j = api.get_json("/v1/agents")
    to_delete: list[tuple[str, str]] = []
    for it in (j.get("agents") or []):
        agent_ref = str(it.get("agent_ref") or "").strip()
        name = str(it.get("name") or "").strip()
        tags = it.get("tags") or []
        if not agent_ref or not name:
            continue
        if isinstance(tags, list) and tag in [str(x).strip() for x in tags]:
            to_delete.append((agent_ref, name))
    if not to_delete:
        print(f"[seed] cleanup: none found for tag={tag}", file=sys.stderr)
        return 0

    for agent_ref, name in to_delete:
        if not apply:
            print(f"[seed] dry-run delete name={name} agent_ref={agent_ref}", file=sys.stderr)
            continue
        st = api.delete("/v1/agents/" + urllib.parse.quote(agent_ref))
        if st < 200 or st >= 300:
            _die(f"delete agent failed status={st} agent_ref={agent_ref} name={name}")
        print(f"[seed] deleted name={name} agent_ref={agent_ref}", file=sys.stderr)
    print(f"[seed] cleanup count={len(to_delete)} apply={apply}", file=sys.stderr)
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default="http://127.0.0.1:8080")
    ap.add_argument("--timeout", type=int, default=30)
    ap.add_argument("--tag", default="preset:role-agents:v1")
    ap.add_argument("--apply", action="store_true", help="Actually create/delete; otherwise dry-run.")
    ap.add_argument("--cleanup", action="store_true", help="Delete agents created by this tool (matched by --tag).")
    args = ap.parse_args()

    api_key = _env_required("ADMIN_API_KEY")
    api = API(base_url=str(args.base_url), api_key=api_key, timeout_s=int(args.timeout))

    tag = str(args.tag).strip()
    if not tag:
        _die("empty --tag")

    if args.cleanup:
        return _cleanup_agents(api, tag=tag, apply=bool(args.apply))

    presets = _role_presets(tag=tag)
    return _create_agents(api, presets=presets, apply=bool(args.apply))


if __name__ == "__main__":
    raise SystemExit(main())

