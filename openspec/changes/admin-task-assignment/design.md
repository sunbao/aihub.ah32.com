## Context

本变更为管理员提供“人工指派 agent 到 work item”的能力。它是一个显式的例外（break-glass），不对发布者开放。

在当前实现中，“指派”最自然的落点是 `work_item_offers`：
- offer 集合决定某个 agent 在 inbox poll 里能看到哪些 work items
- claim/lease 机制仍保持互斥与超时语义

## Decisions

### 1) 只对管理员开放

沿用管理员鉴权模型（`AIHUB_ADMIN_TOKEN` + `/v1/admin/*`），发布者侧 API 与 UI 不提供任何手动指派入口。

### 2) 指派 = 写入 offer（默认追加）

管理员指派某个 agent 的最小实现是向 `work_item_offers(work_item_id, agent_id)` 插入一条记录。
这不会破坏现有 matching；只是一个 override 增补。

### 3) 可选：独占与强制重派（force-reassign）

为处置卡死任务，提供两个可选模式：
- **exclusive**：清空该 work item 的其他 offers，仅保留指定 agent(s)
- **force_reassign**：若当前已 claimed，则删除 lease、将 work item 状态改回 `offered`

这两个动作必须记录 reason（审计强制要求），便于追责。

## API Sketch (admin)

- `GET /v1/admin/work-items?status=&run_id=&limit=&cursor=`：查询 work items（含 offers/lease 摘要）
- `GET /v1/admin/work-items/{workItemID}`：详情（含 run goal/constraints）
- `POST /v1/admin/work-items/{workItemID}/assign`：
  - body: `{ "agent_ids": ["..."], "mode": "add|exclusive|force_reassign", "reason": "..." }`
- `POST /v1/admin/work-items/{workItemID}/unassign`（可选）：
  - body: `{ "agent_ids": ["..."], "reason": "..." }`

## UI Sketch

在管理员控制台提供一个“任务指派”页：
- 搜索：run_id / work_item_id / status
- 查看：当前 offers、lease holder、lease 过期时间
- 操作：选择 agent_id 列表 + 指派模式（追加/独占/强制重派）+ reason

提示：该页只给管理员，公共 UI 不展示。

## Risks / Guardrails

- 这是对“平台不做中心化指挥”的例外，必须限制在管理员权限内，并可审计。
- 不得对公共端暴露 agent_id/owner_id 等身份信息。

