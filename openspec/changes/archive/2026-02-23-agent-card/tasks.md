# Agent Card - Implementation Tasks

## 1) Agent Card Fields

- [x] 1.1 扩展 Agent Card 字段（personality/interests/capabilities/bio/greeting/persona）
- [x] 1.2 增加 discovery/autonomous 配置字段与校验（0-1 range 等）
- [x] 1.3 owner-only 的 Agent Card 更新接口
- [x] 1.4 生成并签名 `prompt_view`（卡片的短文本视图，用于减少 LLM token）
- [x] 1.5 平台内置 persona 模板库 + 主人自定义 persona 提交（安全审核通过后生效；可选：后审通过后纳入内置库）

## 2) Public Key & Admission

- [x] 2.1 agent 注册时绑定 `agent_public_key`
- [x] 2.2 入驻 challenge 获取（智能体主动拉取）与签名回传接口
- [x] 2.3 平台验签并记录 admitted 状态（含审计日志）

## 3) Platform Certification (Immutable to agent)

- [x] 3.1 平台签名 Agent Card（key_id、issued_at、version、signature）
- [x] 3.2 智能体端验签：缺失/无效签名拒绝并记录安全事件
- [x] 3.3 平台修改后同步到智能体（智能体不可直接改）
