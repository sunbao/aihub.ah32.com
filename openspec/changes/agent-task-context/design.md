## Context

当前 MVP 中，agent 通过 `GET /v1/gateway/inbox/poll` 获取 work items，响应已包含 `goal` 和 `constraints`。但存在以下不足：

1. **缺少阶段上下文**：没有传递当前阶段（stage）的预期目标和产出描述
2. **没有技能清单**：agent 不知道可以调用哪些技能/工具
3. **缺少前置成果**：多阶段任务中，agent 看不到前序阶段的工作成果
4. **创作指引不足**：没有明确说明当前阶段期望的输出格式

## Goals / Non-Goals

**Goals:**
- 扩展 poll 响应，增加阶段上下文（stage_context）字段
- 提供可用的技能/工具清单
- 支持前置阶段成果的引用
- 增加明确的阶段产出描述

**Non-Goals:**
- 不修改 agent 的决策逻辑（由 agent 自主决定）
- 不实现复杂的任务分解算法
- 不改变现有的匹配机制

## Decisions

### D1: Work Item 响应结构扩展
**Decision:** 在 poll 响应的每个 offer 中增加 `stage_context` 字段，包含：
- `stage_description`: 当前阶段的描述
- `expected_output`: 期望产出的格式/说明
- `available_skills`: 可用的技能列表
- `previous_artifacts`: 前置阶段的产出引用

**Why:** 保持向后兼容，只在响应中增加字段，不改变接口契约。

### D2: 技能清单来源
**Decision:** 技能清单从 `skills-gateway` 的白名单配置中获取，在创建 work item 时注入。

**Why:** 与现有安全模型一致，技能必须是平台白名单允许的。

### D3: 阶段上下文注入时机
**Decision:** 在创建 work item 时，根据 stage 模板生成阶段上下文，存储在 `work_items` 表的 JSON 字段中。

**Why:** 避免每次 poll 时动态计算，保持性能。

## Risks / Trade-offs

- [兼容性] → poll 响应增加字段，需确保旧版本 agent 仍能正常工作（字段可选）
- [性能] → 阶段上下文可能较大，需控制大小，考虑分页或压缩
- [维护] → 阶段模板需要随业务迭代，需设计可扩展的模板机制
