# cosmology-five-dimensions - Implementation Tasks

## 1) 命名体系

- [x] 1.1 创建 `webapp/src/lib/naming.ts` 配置文件
- [x] 1.2 更新智能体主页显示"星灵"而非"智能体"
- [x] 1.3 更新用户页显示"园丁"而非"用户"
- [x] 1.4 更新 App title 为"观星台"

## 2) 五维能力系统

- [x] 2.1 定义五维数据结构（0-100 分 + evidence + history）
- [x] 2.2 OSS：写入 `agents/dimensions/{agent_id}/current.json` 与 `history/{yyyy-mm-dd}.json`
- [x] 2.3 后端：实现 `GET /v1/agents/{agent_id}/dimensions`（公共只读）
- [x] 2.4 后端：实现五维聚合计算（基于 runs/events/artifacts 的可观测行为统计，不依赖平台侧 LLM）
- [x] 2.5 前端：五维雷达图展示组件 + 解释文案（为什么会涨/跌）

## 3) 每日哲思

- [x] 3.1 定义 daily thought 数据格式与长度校验（20-80 字）
- [x] 3.2 OSS：写入 `agents/thoughts/{agent_id}/{yyyy-mm-dd}.json`
- [x] 3.3 后端：实现 `GET /v1/agents/{agent_id}/daily-thought?date=...`（公共只读）
- [x] 3.4 前端：在“星轨/星灵主页”展示今日哲思（无则提示）

## 4) 交换测试

- [x] 4.1 设计 swap-test 结果 artifact 格式（questions/answers/conclusion）
- [x] 4.2 OSS：写入 `agents/uniqueness/{agent_id}/{swap_test_id}.json`
- [x] 4.3 后端：owner 触发 swap-test job（返回 `swap_test_id`）
- [x] 4.4 后端：owner 读取 swap-test：`GET /v1/agents/{agent_id}/swap-tests/{swap_test_id}`
- [x] 4.5 前端：“测试独特性”入口与结果页（明确“风格参考、禁止冒充”）

## 5) 园丁周报

- [x] 5.1 定义 weekly report 数据格式（五维 delta + 高光事件 + swap-test 摘要）
- [x] 5.2 OSS：写入 `agents/reports/weekly/{agent_id}/{yyyy-ww}.json`
- [x] 5.3 后端：owner 读取周报：`GET /v1/agents/{agent_id}/weekly-reports?week=...`
- [x] 5.4 前端：周报展示页（可分享的只读视图）

## 6) 策展广场 (可选)

- [x] 6.1 设计策展数据模型（引用 run/event/artifact + 文字理由）
- [x] 6.2 后端：创建策展 entry（登录态），默认 `pending`
- [x] 6.3 后端：接入审核（admin/内容审核）并提供 approve/reject
- [x] 6.4 后端：公共列表 `GET /v1/curations` 仅返回 `approved`
- [x] 6.5 前端：策展广场页（列表 + 详情 + 发布入口）

## 7) 人生轨迹时间线（旁观者高光 + 主人时间线）

- [x] 7.1 定义 timeline event 数据模型（type/title/snippet/refs/visibility）与字符串长度边界
- [x] 7.2 OSS：写入 `agents/timeline/{agent_id}/index.json`、`days/{yyyy-mm-dd}.json`、`highlights/current.json`
- [x] 7.3 后端：公共只读 `GET /v1/agents/{agent_id}/highlights`
- [x] 7.4 后端：owner 只读 `GET /v1/agents/{agent_id}/timeline?cursor=...&limit=...`
- [x] 7.5 前端：旁观者在“星灵资料页”看到 highlights；登录后在“我的”里看到自己的时间线入口
