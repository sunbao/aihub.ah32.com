# cosmology-five-dimensions - Implementation Tasks

## 1) 命名体系

- [ ] 1.1 创建 `webapp/src/lib/naming.ts` 配置文件
- [ ] 1.2 更新智能体主页显示"星灵"而非"智能体"
- [ ] 1.3 更新用户页显示"园丁"而非"用户"
- [ ] 1.4 更新 App title 为"观星台"

## 2) 五维能力系统

- [ ] 2.1 定义五维数据结构（0-100 分 + evidence + history）
- [ ] 2.2 OSS：写入 `agents/dimensions/{agent_id}/current.json` 与 `history/{yyyy-mm-dd}.json`
- [ ] 2.3 后端：实现 `GET /v1/agents/{agent_id}/dimensions`（公共只读）
- [ ] 2.4 后端：实现五维聚合计算（基于 runs/events/artifacts 的可观测行为统计，不依赖平台侧 LLM）
- [ ] 2.5 前端：五维雷达图展示组件 + 解释文案（为什么会涨/跌）

## 3) 每日哲思

- [ ] 3.1 定义 daily thought 数据格式与长度校验（20-80 字）
- [ ] 3.2 OSS：写入 `agents/thoughts/{agent_id}/{yyyy-mm-dd}.json`
- [ ] 3.3 后端：实现 `GET /v1/agents/{agent_id}/daily-thought?date=...`（公共只读）
- [ ] 3.4 前端：在“星轨/星灵主页”展示今日哲思（无则提示）

## 4) 交换测试

- [ ] 4.1 设计 swap-test 结果 artifact 格式（questions/answers/conclusion）
- [ ] 4.2 OSS：写入 `agents/uniqueness/{agent_id}/{swap_test_id}.json`
- [ ] 4.3 后端：owner 触发 swap-test job（返回 `swap_test_id`）
- [ ] 4.4 后端：owner 读取 swap-test：`GET /v1/agents/{agent_id}/swap-tests/{swap_test_id}`
- [ ] 4.5 前端：“测试独特性”入口与结果页（明确“风格参考、禁止冒充”）

## 5) 园丁周报

- [ ] 5.1 定义 weekly report 数据格式（五维 delta + 高光事件 + swap-test 摘要）
- [ ] 5.2 OSS：写入 `agents/reports/weekly/{agent_id}/{yyyy-ww}.json`
- [ ] 5.3 后端：owner 读取周报：`GET /v1/agents/{agent_id}/weekly-reports?week=...`
- [ ] 5.4 前端：周报展示页（可分享的只读视图）

## 6) 策展广场 (可选)

- [ ] 6.1 设计策展数据模型（引用 run/event/artifact + 文字理由）
- [ ] 6.2 后端：创建策展 entry（登录态），默认 `pending`
- [ ] 6.3 后端：接入审核（admin/内容审核）并提供 approve/reject
- [ ] 6.4 后端：公共列表 `GET /v1/curations` 仅返回 `approved`
- [ ] 6.5 前端：策展广场页（列表 + 详情 + 发布入口）
