# Agent Task Context - Implementation Tasks

## 1. Database Schema Extension

- [ ] 1.1 Add `context` JSONB column to `work_items` table for storing stage_context
- [ ] 1.2 Add `available_skills` JSONB column to `work_items` table
- [ ] 1.3 Add `review_context` JSONB column for cross-agent review
- [ ] 1.4 Create migration script for schema changes

## 2. Work Item Creation with Context

- [ ] 2.1 Update work item creation logic to include goal and constraints from run
- [ ] 2.2 Implement stage context generation based on stage templates
- [ ] 2.3 Populate available_skills from skills gateway whitelist
- [ ] 2.4 Add previous_artifacts references for multi-stage runs
- [ ] 2.5 Add review_context when creating review-type work items (target_artifact, target_author_tag, review_criteria)
- [ ] 2.6 Support review stage in task orchestration (assign reviewer to another agent's artifact)
- [ ] 2.7 Implement role-based context differentiation (creator gets full context, reviewer gets summarized)
- [ ] 2.8 Add output length specification in expected_output (e.g., "100-200 words")
- [ ] 2.9 Define output format templates per stage (plain text, markdown, JSON)

## 2.1 Scheduled Execution

- [ ] 2.10 Add `scheduled_at` timestamp column to work_items table
- [ ] 2.11 Support creating work items with future scheduled_at time
- [ ] 2.12 Add "scheduled" status for pending scheduled work items
- [ ] 2.13 Implement scheduler worker to transition "scheduled" → "offered" when time arrives
- [ ] 2.14 Update poll query to filter out not-yet-due scheduled items

## 3. Poll Endpoint Enhancement

- [ ] 3.1 Extend poll query to fetch work_item context fields
- [ ] 3.2 Update offerDTO struct with new context fields
- [ ] 3.3 Serialize stage_context, available_skills, previous_artifacts in response
- [ ] 3.4 Serialize review_context in response for review-type work items

## 4. Skills Discovery Endpoint (Optional)

- [ ] 4.1 Create `/gateway/work-items/{workItemID}/skills` endpoint
- [ ] 4.2 Return skills list scoped to work item context

## 5. Testing

- [ ] 5.1 Update unit tests for poll response structure
- [ ] 5.2 Add integration test for stage context propagation
- [ ] 5.3 Verify backward compatibility with existing agents
- [ ] 5.4 Add test for cross-agent review context propagation
- [ ] 5.5 Test review feedback emission to collaboration stream

## 6. OpenClaw Connector Update

Note: 一键安装机制是修改 ~/.openclaw/openclaw.json 中的 skills.entries，不需要修改 OpenClaw 核心代码

- [ ] 6.1 Update `openclaw/skills/aihub-connector/SKILL.md`:
  - Add parsing of new poll response fields: stage_context, expected_output, review_context
  - Add instruction: read expected_output.length and comply with output limits
  - Add instruction: when review_context exists, produce review feedback instead of creation
  - Add instruction: read available_skills from poll response
- [ ] 6.2 Test full flow: poll → claim → emit → submit → complete with new context fields
