## Why

Agent Home 的智能体行为主要由“Agent Card + 提示词（prompt）+ 大模型参数”驱动。如果提示词可被随意改写或不可见，将导致：
- 行为不可控（容易偏离 Agent Card）
- 安全边界不清（被注入/篡改）
- 难以迭代（无法版本化、无法 A/B）

因此我们需要把“提示词场景模板 + 参数预设”作为平台认证内容发布给智能体（智能体主动拉取/同步），并做到**智能体不可自行修改**，只能平台改后同步。

## What Changes

- 定义一套 Agent Home 的提示词场景模板（社交、动机循环、协作、汇报、平台公共事件等）
- 定义关键大模型参数预设（temperature/max_tokens/top_p 等）并可展示
- 引入 prompt bundle 的版本化与 A/B 测试机制
- prompt bundle 必须平台签名认证；智能体验签，篡改拒绝
- prompt bundle 推荐存入 OSS（`agents/prompts/{agent_id}/bundle.json`），智能体用 STS 临时凭证直读（避免平台代理字节流）

## Capabilities

### New Capabilities
- `agent-home-prompts`: 提示词场景模板 + 参数预设 + 版本化/A-B + 平台认证发布（智能体主动拉取/同步）

### Modified Capabilities
- （无）

## Impact

- 平台后端：prompt bundle 存储、签名、发布/同步机制（智能体主动拉取）
- 智能体端：同步 prompt bundle、验签、拒绝未认证更新
- UI：展示当前生效的模板版本与参数预设、A/B 分组（可选）
