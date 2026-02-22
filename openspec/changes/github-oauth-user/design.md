## Context

目标是把“创建用户”改为 GitHub OAuth，并把 GitHub 昵称/头像作为 UI 的用户标识；同时遵循 UI 原则：不展示任何编号/UUID（用户与管理员都不看）。

当前系统鉴权模型：
- user/agent 都通过 `Authorization: Bearer <API key>` 鉴权（服务端仅存 hash）
- UI 侧把 key 存在浏览器本地存储（MVP）

本变更尽量不改变既有鉴权模型，只把“用户创建/绑定”入口替换为 OAuth（首期 GitHub），并为后续扩展其他提供方预留扩展点。

## Decisions

### 1) 认证方式：OAuth 提供方（首期 GitHub；无组织限制）

**Decision:**
- 创建用户必须通过 OAuth 完成（首期 GitHub）
- 彻底取消匿名创建用户
- 不做 org/repo 成员限制

**Why:** 让用户有稳定身份标识，同时保持开放性；并为后续接入其他提供方提供统一模式。

### 2) 用户标识：展示 GitHub 昵称/头像

**Decision:** 控制台 UI 展示 GitHub `login`（或 `name` 优先）+ `avatar_url` 作为用户标识；不展示任何内部 ID/UUID。  
**Why:** 用户关心的是“我是谁”，不是“我的编号是什么”。

### 3) 管理员鉴权不变

**Decision:** 管理员仍使用 `AIHUB_ADMIN_TOKEN`；GitHub 身份不参与管理员鉴权。  
**Why:** 最小改动，避免把运维权限与第三方账号耦合。

### 4) 保持 API Key 鉴权模型（MVP），但 UI 不展示/不复制 key

**Decision:**
- OAuth 成功后生成/轮换 AIHub 用户 API Key，并自动写入浏览器本地存储供 UI 调用 API
- UI 不展示、不回显、不提供“复制用户 key”的入口（用户与管理员都不看）
- 若用户换设备或清空浏览器数据：重新 OAuth 即可恢复使用（会覆盖本地存储中的 key）

**Why:** 用户 key 是纯技术信息，复制对普通用户没有价值；但保留 key 模型可最大化兼容现有后端鉴权实现。

### 5) 安全：state（必要）+ PKCE（建议）

**Decision:** OAuth 流必须校验 `state` 防 CSRF；如实现成本可控，增加 PKCE。  
**Why:** 这是 OAuth 最基本的安全要求；PKCE 能降低泄露风险。

### 6) 前端：手机端优先（Mobile-first）

**Decision:**
- 所有控制台页面按手机端浏览设计（单列布局、少输入、按钮易点、信息密度低）
- “状态/下一步”优先于“技术细节”
- 中文界面为主，避免中英文混搭

**Why:** 真实使用场景以手机查看为主；手机端做好后，桌面端自然可用。

## Flow Sketch

```
用户在 /ui/user.html 点击「用 GitHub 创建用户」
  → GET /v1/auth/github/start（首期）
  → 302 跳转到 GitHub 授权页
  ← GitHub 回调 /v1/auth/github/callback?code&state
  → 服务器向 GitHub 换 token，并拉取 /user
  → upsert 用户（按 provider + subject 唯一；GitHub 即 github_user_id）
  → 生成/轮换 AIHub 用户 API Key
  → 返回一个同域页面：把 key 写入浏览器本地存储，并跳转回 /ui/settings.html（或 /ui/user.html）
```

## Data Model Sketch

为支持后续多提供方，优先采用独立 identity 表（示意）：
- `user_identities.provider`（如 `github`）
- `user_identities.subject`（如 GitHub user id，字符串）
- `user_identities.user_id`（关联 users）
- `user_identities.login/name/avatar_url/profile_url`（用于展示）
- 对 `(provider, subject)` 加唯一约束

（实现也可以选择把字段直接加在 `users` 上，但会降低扩展性；若这么做，需要明确后续迁移路径。）

首期 GitHub 只需要上述展示字段；不要求持久化 GitHub access token（用完即弃）。

## API Sketch

- `GET /v1/auth/github/start`：发起 OAuth（302）
- `GET /v1/auth/github/callback`：回调（换 token / upsert user / 生成 key / 写本地存储后跳转）
- （可选）`GET /v1/me`：返回当前 user 的 GitHub 标识信息（供 UI 展示/校验）

## UI Notes

- 页面只展示：头像、昵称、以及“已完成/未完成”的状态信息（不展示 key、不展示编号）
- 不显示任何编号/UUID；不在 toast/notice 中回显编号
- 所有调试信息写入后端日志或审计表，必要时由开发者查看

## Shadcn UI

**Decision:** 控制台 UI 的视觉与组件风格以 `shadcn-ui/ui` 为准（中文化文案、手机端优先）。  
**Note:** shadcn-ui 主要面向 React + Tailwind；当前项目是 Go embed 静态页。实现阶段需要明确：
- 引入前端构建（React/Tailwind/shadcn）并产出静态资源；或
- 保持静态页，仅借鉴 shadcn 的视觉规范与交互模式（不引入 React）。

实现阶段需要对上述两种方案做取舍并更新本设计。
