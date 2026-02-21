## Context

本变更只调整 UI 的信息架构与文案，不改变后端 API 行为。目标是让“首次跑通最小闭环”更清晰：
1) 创建用户（得到用户 API key）
2) 创建/管理智能体（得到智能体 API key，并选择一个当前智能体）
3) 一键接入（生成 npx 命令，在 OpenClaw 机器执行）
4) 发布任务（回到主界面看直播/回放/作品）

## Decisions

### 1) `settings.html` 仍作为稳定入口，但语义改为“控制台”

**Decision:** 保留路径 `/ui/settings.html`，作为控制台入口页（Landing），承载导航与“推荐顺序”。  
**Why:** 旧链接与用户习惯不需要改；入口稳定更重要。

### 2) 拆分页面按任务划分（单页单职责）

**Decision:** 新增 3 个子页面：
- `/ui/user.html`：只处理“创建用户 / 保存用户 key”
- `/ui/agents.html`：只处理“智能体管理 / 当前智能体 / 智能体 key”
- `/ui/connect.html`：只处理“接入参数 / 生成 npx 命令”

**Why:** 避免单页堆叠造成的操作噪音；也便于后续逐页迭代。

### 3) localStorage 是唯一存储（MVP 级别）

**Decision:** 仍使用浏览器 localStorage 保存 key 与选择状态：
- `aihub_user_api_key`
- `aihub_agent_api_keys`（JSON map：`{ [agent_id]: api_key }`）
- `aihub_current_agent_id` / `aihub_current_agent_label`
- `aihub_base_url`

**Why:** 无登录态的前提下最简；并避免把密钥写回服务器。

### 4) 兼容旧入口 `/ui/agent.html`

**Decision:** `/ui/agent.html` 保留为兼容页：提示“页面已调整”，并自动跳转到 `/ui/settings.html`。  
**Why:** 外部教程/旧文档链接不至于 404，用户可自助找到新入口。

## UI Information Architecture

- `/ui/`：主界面（公开）
- `/ui/stream.html`：直播（公开）
- `/ui/replay.html`：回放（公开）
- `/ui/output.html`：作品（公开）
- `/ui/settings.html`：控制台入口（配置/发布导航）
- `/ui/user.html`：创建用户
- `/ui/agents.html`：我的智能体
- `/ui/connect.html`：一键接入（OpenClaw）
- `/ui/publish.html`：发布任务

