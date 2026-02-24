## 1) 配置与密钥

- [x] 1.1 增加 OAuth（GitHub）配置项（client id/secret、回调地址/基址）
- [x] 1.2 更新 `README.md` / `SMOKE_TEST.md`：如何配置 GitHub OAuth
- [x] 1.3 删除匿名创建用户的 UI 与 API（不再允许匿名创建）

## 2) 数据模型

- [x] 2.1 新增 `user_identities`（provider+subject 唯一）或等价实现，满足后续多提供方扩展
- [x] 2.2 保存并返回用于展示的字段（昵称/头像/主页链接等）
- [x] 2.3 提供读接口（例如 `GET /v1/me`）供 UI 展示当前用户标识

## 3) OAuth API

- [x] 3.1 `GET /v1/auth/github/start`：生成 state（与可选 PKCE），302 到 GitHub 授权页
- [x] 3.2 `GET /v1/auth/github/callback`：校验 state，换 token，拉取 GitHub 用户信息，upsert 用户身份并生成/轮换 AIHub 用户 API Key
- [x] 3.3 失败路径：取消授权/换 token 失败/拉取用户信息失败 → 返回中文错误页（不包含任何编号/UUID/key）

## 4) 控制台 UI（创建用户）

- [x] 4.1 更新 `/ui/user.html`：只保留“用 GitHub 创建用户/登录”入口（取消匿名创建），展示头像/昵称
- [x] 4.2 OAuth 成功后自动写入浏览器本地存储并跳转；UI 不展示/不复制用户 key
- [x] 4.3 控制台相关页面统一改为“从本地存储读取登录态”：不要求手动输入/复制用户 key
- [x] 4.4 手机端优先：单列布局、按钮易点、少输入、信息密度低
- [x] 4.5 全页面不展示任何编号/UUID；提示语不回显编号（管理员页面同样原则）

## 5) 冒烟验证

- [x] 5.1 新增/更新冒烟步骤：完成 GitHub OAuth → 控制台识别用户 → 能创建/管理智能体
- [x] 5.2 验证“无编号 UI”原则：页面与提示语不出现 UUID/编号
