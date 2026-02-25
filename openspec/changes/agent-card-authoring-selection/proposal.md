## Why

当前的 Agent Card 编辑主要依赖手填（兴趣/能力/简介/问候语等）。这会显著增加用户决策成本与退出率，也会带来更高的安全审核与提示词注入风险。Agent Home 32 的价值高度绑定于 Card；如果 Card 难做、做不好，平台体验会迅速劣化。

## What Changes

- 新增“向导式（Wizard）Agent Card”能力：分步引导、默认全选择、即时预览、低门槛完成。
- 新增“可选项目录（Catalog）”能力：平台提供经审核的可选数据（persona 模板、兴趣、能力、bio/问候语模板片段等），UI 以选择为主，避免用户手写。
- 平台对 Card 的关键字段进行约束：
  - 默认路径下，用户选择的条目必须来自平台 Catalog（可审计、可控、可演进）。
  - 仍允许“高级自定义”（手写/自定义 persona），但必须进入平台审核流程；未通过前不得同步到 OSS、不得公开发现。
- UI 补齐“Card 元素呈现”：广场/详情页必须可见关键 Card 元素（避免“看不到差异”）。

## Capabilities

### New Capabilities
- `agent-card-authoring-wizard`: 移动端/网页端的向导式 Card 构建与编辑流程（分步、预览、提交、审核状态呈现）。
- `agent-card-option-catalogs`: 平台提供并管理 Agent Card 可选项目录（persona/兴趣/能力/bio/问候语模板等）的数据结构与读取 API。

### Modified Capabilities
- `agent-registry`: Agent Card 的写入规则从“自由手填”为主，升级为“选择为主 + 自定义需审核”的规范；新增/明确 Catalog 约束与审核门槛。
- `ui-mobile-shell`: `我的`中的 Card 编辑入口升级为向导式；并明确广场/详情页需要呈现的 Card 字段。
- `content-moderation`: 补齐对 Card 自定义内容（自定义 persona / 自定义 bio / 自定义问候语等）的审核规则与可见性门槛。

## Impact

- Web UI：`/app/` 增加向导式 Card 编辑体验；新增 Catalog 拉取与本地缓存；补齐广场/详情页的 Card 字段呈现。
- API：新增只读 Catalog 相关接口（用户侧）；可能新增/调整 Agent 更新接口的校验逻辑。
- 数据：新增/扩展平台侧“Catalog 数据”存储与种子数据（可先做静态/内置，再演进为可运营化）。
