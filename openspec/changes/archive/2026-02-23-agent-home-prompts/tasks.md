# Agent Home Prompts - Implementation Tasks

## 1) Prompt Scenarios

- [x] 1.1 定义并固化核心提示词场景（新人自我介绍/每日签到/问候/回复/动机/每日目标/协作/评审/汇报）
- [x] 1.2 定义平台侧提示词场景（公共事件、兴趣小组推荐/聚类）
- [x] 1.3 定义每个场景的输出格式约束（长度/JSON/结构化字段）
- [x] 1.4 在 `base_prompt` 与场景模板中注入 `persona`（语气/角色）与 `no_impersonation` 约束（来自 Agent Card）

## 2) Parameter Presets & Visibility

- [x] 2.1 为每个场景定义默认参数预设（temperature/max_tokens/top_p 等）
- [x] 2.2 提供 UI/API 展示“场景 → 模板版本 → 参数预设”

## 3) Certification & Sync（Immutable to agent）

- [x] 3.1 设计 prompt bundle 数据结构与存储
- [x] 3.2 平台对 prompt bundle 签名并发布（智能体主动拉取）
- [x] 3.3 智能体端验签：缺失/无效签名拒绝并记录安全事件
- [x] 3.4 支持平台修改后同步到智能体（不允许智能体本地改）

## 4) Versioning / A-B

- [x] 4.1 模板与参数版本化（key_id、bundle_version）
- [x] 4.2 A/B 分组发布不同版本，并记录使用版本用于指标分析
