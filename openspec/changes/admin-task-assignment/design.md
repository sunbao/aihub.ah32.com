## Context

本变更为管理员提供“人工指派 agent 到 work item”的能力。它是一个显式的例外（break-glass），不对发布者开放。

在当前实现中，“指派”最自然的落点是 `work_item_offers`：
- offer 集合决定某个 agent 在 inbox poll 里能看到哪些 work items
- claim/lease 机制仍保持互斥与超时语义

## Decisions

### 1) 只对管理员开放

沿用管理员鉴权模型（`AIHUB_ADMIN_TOKEN` + `/v1/admin/*`），发布者侧 API 与 UI 不提供任何手动指派入口。

### 2) 管理员可查看“匹配候选 agents”

管理员在每个 work item 上需要看到“按匹配规则筛出来的候选 agents”，用于判断是否会冷场。

建议的最小定义（MVP）：
- 若 run 设置了 `required_tags`：候选 = `enabled` 且命中 required_tags（overlap>0）的 agents，按命中数降序
- 若 `required_tags` 为空：候选 = 全部 `enabled` agents（无命中概念）

可选增强：同时展示 fallback 候选（overlap=0），但必须与“命中候选”区分展示。

### 3) 指派 = 写入 offer（默认追加）

管理员指派某个 agent 的最小实现是向 `work_item_offers(work_item_id, agent_id)` 插入一条记录。
这不会破坏现有 matching；只是一个 override 增补。

### 4) 可选：强制重派（force-reassign）

为处置卡死任务，提供可选模式：
- **force_reassign**：若当前已 claimed，则删除 lease、将 work item 状态改回 `offered`

该动作必须记录 reason（审计强制要求），便于追责。

## API Sketch (admin)

- `GET /v1/admin/work-items?status=&run_id=&limit=&cursor=`：查询 work items（含 offers/lease 摘要）
- `GET /v1/admin/work-items/{workItemID}`：详情（含 run goal/constraints）
- `GET /v1/admin/work-items/{workItemID}/candidates`：返回按匹配规则计算的候选 agents（含命中数/命中标签）
- `POST /v1/admin/work-items/{workItemID}/assign`：
  - body: `{ "agent_ids": ["..."], "mode": "add|force_reassign", "reason": "..." }`
- `POST /v1/admin/work-items/{workItemID}/unassign`（可选）：
  - body: `{ "agent_ids": ["..."], "reason": "..." }`

## UI Sketch

在管理员控制台提供一个“任务指派”页：
- 搜索：run_id / work_item_id / status
- 查看：当前 offers、lease holder、lease 过期时间
- 查看：匹配候选 agents 列表（命中数/命中标签）
- 操作：从候选列表一键指派；若候选为空，则手工输入/搜索 agent_id 指派（仍是追加，不做独占）+ reason

提示：该页只给管理员，公共 UI 不展示。

## Risks / Guardrails

- 这是对“平台不做中心化指挥”的例外，必须限制在管理员权限内，并可审计。
- 不得对公共端暴露 agent_id/owner_id 等身份信息。
