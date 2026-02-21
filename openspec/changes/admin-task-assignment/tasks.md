## 1) Admin APIs

- [x] 1.1 Admin auth middleware (`AIHUB_ADMIN_TOKEN`)
- [x] 1.2 List/search work items for admin (status/run_id) including offers + lease summary
- [x] 1.3 Work item detail endpoint (includes run goal/constraints)
- [x] 1.4 Candidates endpoint: compute matching agents for a work item (hits + matched_tags + optional missing_tags; group overlap>0 vs overlap=0)
- [x] 1.5 Assign endpoint: add offers (and optional force-reassign) with audit reason
- [x] 1.6 (Optional) Admin list/search agents (for manual selection when candidates empty)
- [x] 1.7 (Optional) Unassign endpoint (remove offers)

## 2) Data / Audit

- [x] 2.1 Add `moderation_actions`-like audit entries for assignment actions (or reuse a shared admin_actions table)
- [x] 2.2 Ensure all admin actions written to `audit_logs`

## 3) UI

- [x] 3.1 Add `/ui/admin-assign.html` (token input + work item search + candidates + assign)
- [x] 3.2 Ensure admin UI not linked from public pages (direct URL only)

## 4) Smoke / Verification

- [x] 4.1 Create a run with no matching candidates (overlap=0) → admin assigns a specific agent → agent polls and sees offer → claim/complete
- [x] 4.2 (Optional) Force-reassign flow: claim by agent A → admin force reassign to agent B → B can claim after lease canceled
