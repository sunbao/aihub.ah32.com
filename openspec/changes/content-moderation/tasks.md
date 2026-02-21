## 1) Data Model

- [x] 1.1 Add review state for `runs`, `events`, `artifacts`: `pending/approved/rejected` (default: pending+visible)
- [x] 1.2 Add `moderation_actions` table for action history (approve/reject/unreject + reason)
- [x] 1.3 Add indexes for queue/search (e.g., status, created_at)

## 2) Admin Auth + API

- [x] 2.1 Add admin auth middleware using `AIHUB_ADMIN_TOKEN`
- [x] 2.2 Add admin endpoints to read raw content (run/event/artifact detail)
- [x] 2.3 Add approve/reject/unreject endpoints + audit logs
- [x] 2.4 Add moderation queue endpoints (pending queue + filters by type/status)

## 3) Public Enforcement (No Leaks)

- [x] 3.1 Runs list filters `rejected` runs
- [x] 3.2 Run detail endpoint redacts rejected goal/constraints (placeholder)
- [x] 3.3 Stream/replay redacts rejected events (placeholder payload, keep seq)
- [x] 3.4 Output/artifact endpoints never return rejected artifact content
- [x] 3.5 If latest artifact is rejected, return placeholder content: “该作品已被管理员审核后屏蔽”
- [x] 3.6 Ensure UI pages use the public endpoints only (no bypass)

## 4) Admin UI

- [x] 4.1 Add `/ui/admin.html` with token input + queue list
- [x] 4.2 Add detail panel + approve/reject actions (reason input)
- [x] 4.3 Add “已拒绝”列表/搜索（管理员可回看问题内容）

## 5) Smoke / Verification

- [x] 5.1 Add/update smoke script: create run → emit event → submit artifact → admin rejects → public UI shows placeholders only
- [x] 5.2 Document moderation behavior in `SMOKE_TEST.md`
