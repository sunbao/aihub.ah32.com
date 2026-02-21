## 1) Data Model

- [ ] 1.1 Add review state for `runs`, `events`, `artifacts`: `pending/approved/rejected` (default: pending+visible)
- [ ] 1.2 Add `moderation_actions` table for action history (approve/reject/unreject + reason)
- [ ] 1.3 Add indexes for queue/search (e.g., status, created_at)

## 2) Admin Auth + API

- [ ] 2.1 Add admin auth middleware using `AIHUB_ADMIN_TOKEN`
- [ ] 2.2 Add admin endpoints to read raw content (run/event/artifact detail)
- [ ] 2.3 Add approve/reject/unreject endpoints + audit logs
- [ ] 2.4 Add moderation queue endpoints (pending queue + filters by type/status)

## 3) Public Enforcement (No Leaks)

- [ ] 3.1 Runs list filters `rejected` runs
- [ ] 3.2 Run detail endpoint redacts rejected goal/constraints (placeholder)
- [ ] 3.3 Stream/replay redacts rejected events (placeholder payload, keep seq)
- [ ] 3.4 Output/artifact endpoints never return rejected artifact content
- [ ] 3.5 If latest artifact is rejected, return placeholder content: “该作品已被管理员屏蔽”
- [ ] 3.6 Ensure UI pages use the public endpoints only (no bypass)

## 4) Admin UI

- [ ] 4.1 Add `/ui/admin.html` with token input + queue list
- [ ] 4.2 Add detail panel + approve/reject actions (reason input)
- [ ] 4.3 Add “已拒绝”列表/搜索（管理员可回看问题内容）

## 5) Smoke / Verification

- [ ] 5.1 Add/update smoke script: create run → emit event → submit artifact → admin rejects → public UI shows placeholders only
- [ ] 5.2 Document moderation behavior in `SMOKE_TEST.md`
