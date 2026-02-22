## Why

当前 MVP 中，agent 通过 poll/claim 获取 work item 后，缺少足够的任务上下文来理解任务目标、选择合适的技能、做出创作决策。平台只提供基础的 work item 结构（stage, kind），但没有传递任务的完整背景信息（如任务目标、约束条件、可用技能列表等）。这导致 agent 难以做出有针对性的创作，影响任务完成质量。

## What Changes

1. **扩展 work item 结构**：在 work item 中增加 `goal`（任务目标）、`constraints`（约束条件）、`stage_context`（阶段上下文）等字段
2. **技能清单机制**：平台提供可用的技能/工具列表，让 agent 知道可以调用什么能力
3. **任务历史上下文**：传递之前阶段的工作成果，帮助 agent 理解任务演进
4. **明确创作指引**：在 work item 中提供当前阶段的期望输出描述

## Capabilities

### New Capabilities
- `task-context-delivery`: 任务上下文传递机制，定义平台如何向 agent 提供足够的任务背景信息
- `skill-discovery`: 技能发现机制，agent 可以查询当前可用的技能列表

### Modified Capabilities
- `skills-gateway`: 修改 poll 响应结构，增加任务上下文字段
- `task-orchestration`: 扩展 work item 模型，支持上下文字段

## Impact

- `internal/httpapi/` - 修改 poll/claim 响应
- `internal/models/` - 扩展 WorkItem 结构
- `openspec/specs/skills-gateway/spec.md` - 更新接口规范
- `openspec/specs/task-orchestration/spec.md` - 更新 work item 模型
