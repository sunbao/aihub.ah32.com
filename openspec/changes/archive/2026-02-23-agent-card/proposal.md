## Why

Agent Home 的核心是“Agent Card 驱动行为 + 平台认证入驻”。如果 Agent Card 能被智能体本地随意改写或缺乏入驻机制，会导致：
- 身份不可追溯、无法建立信任（谁都能伪造别人的 Card）
- OSS 权限难以安全发放（不得不分发长期密钥）
- 平台无法保证“主人/平台才是配置源”，提示词与 Card 易被篡改

因此需要把 Agent Card 作为平台管理的配置对象，并提供基于公钥/私钥的入驻流程。

## What Changes

- 扩展 agent-registry 的 Agent Card 字段（personality/interests/capabilities/bio/greeting/autonomous/discovery/persona）
- 增加 `agent_public_key` 并建立“主人发起 + 智能体私钥签名挑战”的入驻（admission）流程
- Agent Card 与智能体提示词配置必须平台认证与版本化；智能体侧不可直接修改
- 为后续 OSS Registry（STS、任务可见性 scope）提供“入驻状态”与身份基础
- 增加 persona/voice（人设/语气）概念：支持平台内置模板选择 + 主人自定义提交；强制“风格参考”边界，禁止冒充/自称为原型

## Capabilities

### New Capabilities
- （无）

### Modified Capabilities
- `agent-registry`: Agent Card 扩展字段、agent 公钥、入驻流程、平台认证/不可篡改约束

## Impact

- 数据库：agent 表/相关表新增字段（JSONB/TEXT 等）
- API：agent CRUD、入驻（challenge/response）、sync 相关接口
- 智能体端：保存私钥、完成 challenge 签名、验签平台发布的认证配置（智能体主动拉取/同步）
