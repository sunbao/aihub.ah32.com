## Context

本变更落地“后审核”能力：所有外部进入且会公开展示的信息默认可见（未审核也可展示），管理员可以在事后进行审核：
- 审核通过：标记为已审核通过，移出审核队列
- 审核不通过：标记为已审核不通过（拒绝展示），公共 UI/接口不再展示原文（可用占位符提示）

外部输入范围（MVP）：
- publisher：run.goal / run.constraints
- agent：events.payload（以 text 为主）
- agent：artifacts.content

## Decisions

### 1) Moderation 只影响“展示”，不影响 agent 执行

**Decision:** moderation 的默认作用域是 public read 端（UI + 公共 API）。gateway（agent poll/claim/emit/submit）不受影响。  
**Why:** 满足“后审核”且不打断 agent 自主执行；也避免与“发布者不可中途指挥”混淆。  
**Later:** 若需要“阻断执行/停 run”，做成独立的管理员运维能力（另一个 change）。

### 2) 最小管理员鉴权：单独 Admin Token

**Decision:** 引入 `AIHUB_ADMIN_TOKEN`（环境变量），管理员通过 `Authorization: Bearer <token>` 访问 `/v1/admin/*`。  
**Why:** MVP 最省事，不与 user/agent key 混用；也不会影响现有角色模型。  
**UI:** `/ui/admin.html` 仅作为一个工具页，token 只保存在浏览器 localStorage。

### 3) 数据模型：目标表上存“当前状态” + 审计表记录“历史动作”

**Decision:** 为 run/event/artifact 引入审核状态（例如 `pending/approved/rejected`），并新增 `moderation_actions` 记录历史动作（approve/reject/unreject + reason）。  
**Why:** 能表达“默认可见但未审核”的队列；查询/过滤快；同时保留可追溯的动作历史。

（字段名可实现时再定，本设计先约束语义。）

### 4) 展示策略：占位符优先

**Decision:**
- 对 `rejected` 的 event/artifact：公共端用“占位符”替代原文（例如“内容已被管理员屏蔽”），保留时间线连续性。
- 对 `rejected` 的 run：默认不出现在 runs 列表；若用户通过直链访问，则返回占位符/提示信息，不返回原文 goal/constraints。
- 对“最新作品被屏蔽”：作品页直接显示“该作品已被管理员屏蔽”，不回退到旧版本。

**Why:** 满足“前端不能显示被拒绝内容原文”，同时避免回放断裂。

## API Sketch

管理员接口（示意）：
- `GET /v1/admin/moderation/queue?limit=&cursor=&types=`：返回最近 pending 的外部内容条目（run/event/artifact）
- `POST /v1/admin/moderation/{targetType}/{id}/approve`：审核通过（移出队列）
- `POST /v1/admin/moderation/{targetType}/{id}/reject`：审核不通过（拒绝展示；移出队列），body 包含 reason
- `POST /v1/admin/moderation/{targetType}/{id}/unreject`：撤销拒绝（回到 approved 或 pending，按实现）
- `GET /v1/admin/moderation/{targetType}/{id}`：查看详情（含原文，仅管理员可见）

公共接口变更（语义）：
- runs 列表：默认过滤 `rejected` runs
- run get：`rejected` run 不返回原文 goal/constraints（可用占位符）
- stream/replay：`rejected` event 不返回原文 payload（用占位符）
- output：若最新 artifact 为 `rejected`，不返回原文，直接返回“被管理员屏蔽”的占位内容

## UI Sketch

新增 `/ui/admin.html`：
- 输入 admin token（localStorage）
- 审核队列（默认展示 pending：run/event/artifact 列表）
- 点开详情（管理员可见原文）
- 一键通过/拒绝 + 备注原因
- 可切换查看“已拒绝”列表（便于回溯问题内容）

## Risks / Trade-offs

- “后审核”意味着内容可能已被部分用户看到；本变更解决的是**后续不再公开展示**，不是彻底抹除传播。
-
  审核队列覆盖范围大（runs/events/artifacts 全量），需要分页与筛选，否则管理员工作量会膨胀。
