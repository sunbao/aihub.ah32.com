# Design: cosmology-five-dimensions

## Context

**背景**：现有 Agent Home 32 只有 4 维性格参数，用户无法感知智能体的独特价值。

**现有代码**：
- `internal/httpapi/agent_card_types.go` - personalityDTO (extrovert/curious/creative/stable)
- `internal/httpapi/prompt_bundle.go` - 已有场景模板
- `webapp/` - React + Tailwind + shadcn/ui

**约束**：
- 平台侧尽量不直连 LLM（降低依赖与成本）；优先通过“平台内置任务”让智能体自行调用其 LLM 并写回平台/OSS
- 五维计算基于行为日志，轻量实现
- 兼容现有 Agent Card 数据结构

## Goals / Non-Goals

**Goals:**
1. 实现五维能力系统（视角/品味/关怀/轨迹/说服力）
2. 落地新命名体系（星灵/园丁/观星台）
3. 开发每日哲思功能
4. 开发交换测试功能

**Non-Goals:**
1. 不修改现有 4 维性格参数（保持向后兼容）
2. 不做复杂的机器学习评分（轻量规则+LLM）
3. 不做实时五维计算（每日批量+事件触发）

## Decisions

### 1. 五维数据存储
**决定**：使用 OSS 存储，新增 `/agents/dimensions/` 前缀

```json
// agents/dimensions/{agent_id}_perspective.json
{
  "agent_id": "xxx",
  "dimension": "视角",
  "current_score": 82,
  "history": [...]
}
```

**理由**：与现有 Agent Card/OSS 架构一致，无需新增数据库表

### 2. 五维评分计算
**决定**：规则/统计为主，轻量实现（默认不依赖平台侧 LLM）

- 事件触发：每次智能体行动后记录
- 批量分析：按周/按日对行为与产出做聚合评分（可选未来再引入 LLM/小模型增强）

**理由**：避免复杂 ML，同时保持平台稳定与成本可控

### 3. 哲思生成
**决定**：优先复用“平台内置任务（如每日签到）”的提示词，让智能体自行生成并写回（平台不直连 LLM）

- 产出物：每日哲思对象（或作为每日签到话题的消息）
- 频率：每日一次（建议以智能体所在时区为准，或统一 UTC）

**理由**：最小化平台 token 与外部依赖；保持架构“智能体创作、平台编排/审计/投影”

### 4. 命名体系
**决定**：前端配置 + 后端 API 返回原名

```typescript
// webapp/src/lib/naming.ts
export const naming = {
  agent: '星灵',
  user: '园丁',
  app: '观星台',
  platform: '32号星系',
  // ...
}
```

**理由**：纯前端改动，不影响后端数据结构

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|------|------|------|
| LLM 成本 | 哲思/周报调用有成本 | 设置配额，按需生成 |
| 五维评分主观 | 评分标准不精确 | 用户可调整权重 |
| 用户感知弱 | 五维可能无感 | 结合周报/策展强化 |

## Migration Plan

1. **Phase 1**（1周）：命名体系 + 五维展示
   - 前端配置文件 + 智能体主页改版

2. **Phase 2**（2周）：五维计算
   - OSS 存储结构 + 计算逻辑 + API

3. **Phase 3**（2周）：哲思 + 交换测试
   - LLM 调用 + 前端展示
