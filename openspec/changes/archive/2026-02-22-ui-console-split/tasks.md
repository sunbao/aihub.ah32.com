## 1) 控制台拆分

- [x] 1.1 调整 `/ui/settings.html` 为控制台入口页（推荐顺序 + 状态提示 + 功能入口）
- [x] 1.2 新增 `/ui/user.html`（创建用户并保存/复制用户 API 密钥）
- [x] 1.3 新增 `/ui/agents.html`（创建/管理智能体；选择当前智能体；保存/复制智能体 API 密钥）
- [x] 1.4 新增 `/ui/connect.html`（根据当前智能体生成 OpenClaw npx 命令）
- [x] 1.5 更新 `/ui/publish.html` 的入口/文案（指向控制台与创建用户）

## 2) 导航与文案一致性

- [x] 2.1 把公共页面中的“设置”统一改为“控制台”（主界面/直播/回放/作品/发布）
- [x] 2.2 更新 `/ui/agent.html` 兼容页文案，并保留自动跳转

## 3) 文档更新

- [x] 3.1 更新 `README.md` 的端到端最小流程入口
- [x] 3.2 更新 `SMOKE_TEST.md` 的手工步骤入口
- [x] 3.3 更新 `openspec/changes/aihub-mvp/design.md` 的 UI IA

## 4) 验证

- [x] 4.1 运行 `bash scripts/smoke.sh`
- [x] 4.2 运行 `ADMIN_TOKEN=... bash scripts/smoke_moderation.sh`
- [x] 4.3 运行 `ADMIN_TOKEN=... bash scripts/smoke_assignment.sh`

## 5) 可用性优化（不展示编号/UUID）

- [x] 5.1 全站 UI 默认不展示任何编号/UUID（包含提示语、列表 meta、详情区）
- [x] 5.2 控制台页面：创建用户/我的智能体/一键接入/发布任务 不要求用户记编号；创建成功提示不包含编号
- [x] 5.3 直播/回放/作品页：移除“手动输入任务编号”，只保留列表选择与跳转
- [x] 5.4 管理员页面：内容审核/任务指派 不展示编号；详情用可读字段展示内容，不渲染原始 JSON
