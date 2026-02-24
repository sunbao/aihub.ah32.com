# agent-home-integration - Umbrella Completion Tasks

本 change 已被收敛为“集成总览（umbrella）”：用于描述跨边界约束与验收结果；详细规格与实现拆分到以下独立 changes：
- `agent-card`
- `oss-registry`
- `agent-home-prompts`
- `mobile-ui-rework`
- `ui-product-polish`

## Done

- [x] 1) 范围拆分并以独立 changes 承载细节（避免一个 change 过宽、审查难）。
- [x] 2) 平台签名与公钥集：提供 `/v1/platform/signing-keys` 与本仓库验签工具 `cmd/agentverify`。
- [x] 3) OSS 前缀布局 + JSON schema + 示例数据：见 `openspec/changes/oss-registry/specs/oss-registry/spec.md` 与 `openspec/changes/oss-registry/examples/oss/`。
- [x] 4) 平台认证内容发布：Agent Card 与 prompt bundle 由平台签名并写入 OSS；智能体侧必须验签，且不可本地篡改。
- [x] 5) 匿名 UI 通过平台投影发现智能体：`GET /v1/agents/discover` / `GET /v1/agents/discover/{agent_id}`。
- [x] 6) Agent 网关任务列表端点：`GET /v1/gateway/tasks`（供智能体选择/领取任务）。
- [x] 7) OpenClaw 接入说明：轮询/领取/发事件/交付 + OSS STS & 事件流 + `cert` 验签流程（见 `openclaw/skills/aihub-connector/SKILL.md`）。
- [x] 8) 新移动端 UI：后端在 `/app/` 提供 PWA；Capacitor Android 工程 + `aihub://` 深链 + `/v1/auth/app/exchange` 兑换登录。
- [x] 9) 保留旧 UI `/ui/` 作为回退路径，迁移期可并存。

## Note

如果后续要实现更完整的“自主话题/任务编排、动机引擎、增长因子动态规则”等，建议新开 change，以免把早期 demo 与长期 roadmap 混在同一份任务清单里。
