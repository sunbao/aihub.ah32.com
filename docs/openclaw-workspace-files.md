---
title: OpenClaw Workspace Key Files (Notes)
summary: OpenClaw 工作区关键文件解读、相对路径与加载规则（基于 D:\\openclaw 源码）
last_updated: 2026-03-10
---

# OpenClaw 工作区关键文件解读

这份文档用于解释 OpenClaw 在“工作区（workspace）”里会使用哪些关键文件、每个文件的意义、相对路径约定、以及它们在不同会话类型（主会话 / cron / 子智能体）下的加载差异。

本文基于本机 `D:\openclaw` 源码与内置模板整理，便于你后续做平台侧（AIHub）的人设/身份体系决策。

## 1) 工作区目录与相对路径

### 默认工作区目录（绝对路径模式）

- 默认：`~/.openclaw/workspace`
- Profile 非 default 时：`~/.openclaw/workspace-<profile>`

依据：`D:\openclaw\src\agents\workspace.ts` 的 `resolveDefaultAgentWorkspaceDir(...)` 与 `OPENCLAW_PROFILE` 行为。

### 关键文件的相对路径（相对 workspace root）

下面这些路径全部以“工作区根目录”为基准：

- `AGENTS.md`
- `SOUL.md`
- `IDENTITY.md`
- `USER.md`
- `TOOLS.md`
- `HEARTBEAT.md`
- `BOOTSTRAP.md`
- `MEMORY.md`（可选）
- `memory.md`（可选；与 `MEMORY.md` 二选一或同时存在时去重）
- `memory/YYYY-MM-DD.md`（日记型记录；模板建议“今天 + 昨天”）
- `memory/`（目录；用于存放每日记录、状态文件等）

注意：OpenClaw 以“文件名（basename）白名单”来识别哪些文件能作为 bootstrap/context 文件加载；拼写错误（例如 `indetity.md`）不会被识别。

## 2) 文件逐一解读（意义 + 推荐内容边界）

### `SOUL.md`：你是谁（人格/语气/边界）

**意义**

- 定义智能体的“人格与行为原则”，属于最核心的人设文件。
- 影响面最大：OpenClaw 会在 system prompt 中明确要求“若存在 SOUL.md，应体现其 persona 与语气，避免僵硬/模板化回复”。

**建议内容**

- 你希望智能体如何表达（简洁/严谨/幽默/温暖等）。
- 必须遵守的边界（隐私、对外发言谨慎、不要冒充用户等）。
- 一些“长期不变”的价值准则（优先行动、先自查再提问、以能力赢得信任等）。

**风险提示**

- 不建议把会变动的具体执行任务（尤其带敏感信息/凭证/个人隐私）长期写在 SOUL.md；它更适合长期准则。

模板参考：`D:\openclaw\docs\reference\templates\SOUL.md`。

### `IDENTITY.md`：身份元信息（名字/emoji/头像/气质）

**意义**

- 用于描述“你作为智能体的身份外观层信息”，更偏 UI/展示与轻量人格标签。
- OpenClaw 会解析常见字段（例如 Name / Emoji / Avatar / Vibe 等），并且会忽略模板占位符（防止用户没改模板导致无意义字段污染）。

**建议内容**

- `Name`：对外显示名（更像“角色名/代号”，不建议写真人姓名）。
- `Emoji`：你的标识。
- `Avatar`：头像（支持 workspace 相对路径或 URL / data URI）。
- `Vibe/Creature`：轻量风格标签。

模板参考：`D:\openclaw\docs\reference\templates\IDENTITY.md`。

解析依据：`D:\openclaw\src\agents\identity-file.ts`。

### `USER.md`：关于你的真人用户（称呼/时区/偏好）

**意义**

- 记录“你正在帮助的那个人”的背景信息与偏好，用于提升长期协作体验。
- 本质上是“隐私敏感文件”：内容往往属于真人用户画像。

**建议内容**

- 如何称呼用户、时区、沟通偏好。
- 他们在做什么项目、喜欢什么表达方式、讨厌什么风格（避免踩雷）。

**风险提示（非常重要）**

- 不建议把这类内容同步到任何“公共可发现/可审核/可分享”的平台对象里。
- 如果你未来做 AIHub 平台的引流/广场匿名浏览，`USER.md` 这种类型的内容应当始终本地化，避免跨边界传播。

模板参考：`D:\openclaw\docs\reference\templates\USER.md`。

### `AGENTS.md`：工作区规则与“每次开工流程”

**意义**

- 这是 OpenClaw 工作区的“运行手册”：告诉智能体每次会话启动要先读什么、怎么记忆、群聊怎么发言、外部动作何时要先问。
- 它不是“人设本体”，但它定义了你怎么工作，是稳定性与安全性的关键。

**建议内容**

- 每次启动先做的固定步骤（读 SOUL/USER/memory）。
- 群聊策略（什么时候保持沉默，避免刷屏）。
- 数据安全红线（别外传，别做破坏性动作等）。
- 记忆维护策略（daily logs -> 汇总到 MEMORY）。

模板参考：`D:\openclaw\docs\reference\templates\AGENTS.md`。

### `TOOLS.md`：本地环境备忘录（非“工具权限”）

**意义**

- 这是“你这台机器/环境”的特殊信息记录处：摄像头名、SSH 主机别名、TTS 偏好等。
- 重要点：它不会决定“工具是否可用”，只是帮助你更好地使用外部工具。

模板参考：`D:\openclaw\docs\reference\templates\TOOLS.md`。

### `HEARTBEAT.md`：心跳任务清单（主动模式）

**意义**

- 用于心跳/主动轮询时的任务清单（例如定期检查 inbox、日程等）。
- 模板明确建议：保持为空或只有注释时，可以跳过心跳调用以节省成本。

模板参考：`D:\openclaw\docs\reference\templates\HEARTBEAT.md`。

### `BOOTSTRAP.md`：新工作区首次引导（“出生证明”）

**意义**

- 新工作区第一次启动时的引导脚本，用来让用户快速填好 `IDENTITY.md`、`USER.md`，并引导一起编辑 `SOUL.md`。
- 常见做法是“完成引导后删除 BOOTSTRAP.md”，避免之后每次启动都带着“新手引导”语义。

模板参考：`D:\openclaw\docs\reference\templates\BOOTSTRAP.md`。

源码依据：`D:\openclaw\src\agents\workspace.ts` 的 `ensureAgentWorkspace(...)`（会记录 onboarding 完成状态，并避免重复创建）。

### `MEMORY.md` / `memory.md` / `memory/YYYY-MM-DD.md`：记忆体系

**意义**

- `memory/YYYY-MM-DD.md`：每日原始记录（事件流水、今天发生了什么）。
- `MEMORY.md` / `memory.md`：长期记忆（提炼后的稳定信息：偏好、决策、长期项目脉络）。

**行为要点**

- `MEMORY.md` 与 `memory.md` 都被认可为长期记忆文件名；如果两个都存在，OpenClaw 会按真实路径去重，避免重复注入同一份内容。
- 在某些上下文（群聊/共享场景）里，OpenClaw 会避免注入长期记忆（模板也强调这是“安全”考虑）。

源码依据：`D:\openclaw\src\agents\workspace.ts:461`（memory entries 解析与去重），以及相关测试 `D:\openclaw\src\agents\workspace.test.ts`。

## 3) 加载与过滤规则（为什么这很重要）

### OpenClaw 会把这些文件“注入到系统提示词上下文”

- 这些文件在运行时会以“Project Context files”的形式拼进 system prompt。
- 并且对 `SOUL.md` 会额外强调：存在则应体现其 persona/tone。

源码依据：`D:\openclaw\src\agents\system-prompt.ts`（Project Context 构建逻辑）。

### 不同会话类型会过滤（主会话 vs cron / 子智能体）

OpenClaw 会根据 sessionKey 识别“子智能体/cron”等场景，并限制可注入的文件集合，核心目的通常是：

- 防止把私密信息（尤其长期记忆）注入到共享上下文中
- 限制 token 消耗

示例行为（概念层面）：

- 主会话：注入上述大部分文件（包含 HEARTBEAT/BOOTSTRAP/可用的 MEMORY）。
- cron / 子智能体：只保留少数核心文件（例如 AGENTS/TOOLS/SOUL/IDENTITY/USER），并排除 HEARTBEAT/BOOTSTRAP/MEMORY。

源码依据：`D:\openclaw\src\agents\workspace.ts` 的 `filterBootstrapFilesForSession(...)` 与测试用例。

### “额外 bootstrap 文件”不是任意文件：只允许白名单文件名

OpenClaw 支持通过额外 pattern 加载 bootstrap/context 文件，但它会强制要求“basename 必须是认可的 bootstrap 文件名集合”，避免任意读文件成为安全问题。

源码依据：`D:\openclaw\src\agents\workspace.ts` 的 `loadExtraBootstrapFilesWithDiagnostics(...)`。

## 4) 与 AIHub 平台化设计相关的结论（供你做决策时参考）

这套文件体系是“本地工作区上下文体系”，强项是：

- 适合单机/单用户长期协作（尤其 SOUL/USER/MEMORY）
- 可由用户自由编辑、逐步沉淀

但它也天然带来平台化风险：

- `USER.md` / `MEMORY.md` 等容易包含个人隐私，不适合做公开展示或跨边界传播
- Markdown 自由度高，平台侧要做审核/认证/发现会更难（最终还是会走向结构化摘要与版本管理）

如果你后续要讨论“AIHub 的 Agent Card 要不要删、如何并存”，建议把 OpenClaw 文件体系定位为：

- 本地输入与沉淀：SOUL/IDENTITY/USER/MEMORY 等
- 平台展示与运行时权威：平台侧应当有可审核、可版本化、可签名的结构化对象（具体形态后续再讨论）

