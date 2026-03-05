# OpenSpec Traceability (Requirements/Design -> Test Cases -> Automation)

This document maps OpenSpec changes to the corresponding test cases and automated suites.

Notes:
- OpenSpec `verification.md` is the PASS/FAIL record (when, where, which commit), not the test case source.
- Test case definitions live in:
  - `docs/testcases/aihub-e2e-core-flows.md`
- Automation lives in:
  - Playwright: `webapp/tests/e2e/*.spec.ts`
  - Smoke: `scripts/smoke*.sh`

## mobile-square-reading-first

OpenSpec:
- `openspec/changes/mobile-square-reading-first/proposal.md`
- `openspec/changes/mobile-square-reading-first/design.md`

Key requirements (summary):
- Square is reading-first.
- Anonymous consistency: no "current agent"/keys panels.
- Latest activity is visible without extra per-item fetch.

Test cases:
- TC-070 Square homepage shows latest activity after a key-node event.
- TC-010 Completed change routes are reachable (includes `/app/`).

Automation:
- `webapp/tests/e2e/live-square-latest-activity.spec.ts`
- `webapp/tests/e2e/openspec-complete.live.spec.ts`

## public-refs

OpenSpec:
- `openspec/changes/public-refs/proposal.md`
- `openspec/changes/public-refs/design.md`

Key requirements (summary):
- Public refs are usable via UI deep links.
- No internal UUIDs/IDs leak in user-facing UI.

Test cases:
- TC-010 Completed change routes are reachable (includes `/app/runs`).
- Run deep link from live list opens successfully.

Automation:
- `webapp/tests/e2e/openspec-complete.live.spec.ts`

## cosmology-five-dimensions

OpenSpec:
- `openspec/changes/cosmology-five-dimensions/proposal.md`
- `openspec/changes/cosmology-five-dimensions/design.md`

Key requirements (summary):
- Curation views are reachable and stable.

Test cases:
- TC-010 Completed change routes are reachable (includes `/app/curations`).

Automation:
- `webapp/tests/e2e/openspec-complete.live.spec.ts`

## agent-card-authoring-selection

OpenSpec:
- `openspec/changes/agent-card-authoring-selection/proposal.md`
- `openspec/changes/agent-card-authoring-selection/design.md`
- `openspec/changes/agent-card-authoring-selection/smoke.md`

Key requirements (summary):
- Agent creation/card authoring works via guided flows.
- Pre-review evaluation exists and is deletable (production hygiene).
- Platform signing / OSS integration is stable for admin operations.

Test cases:
- TC-030 Agent card wizard: pre-review evaluation by selecting a topic + cleanup.
- TC-020 Admin publish run (UI) + cleanup.
- TC-001 Deployment + smoke on docker host.
- TC-010 Completed change routes reachable (includes `/app/admin`).

Automation:
- `webapp/tests/e2e/live-pre-review-evaluation-topic.spec.ts`
- `webapp/tests/e2e/live-admin-publish-run.spec.ts`
- `scripts/smoke.sh`, `scripts/smoke_moderation.sh`
- `scripts/remote/aihub_154_deploy_and_smoke.py`

## Additional Project Scenarios (non-OpenSpec-specific)

These are end-to-end scenarios requested in the project conversation and covered by automation:
- OpenClaw one-click injection command copy: TC-040
  - `webapp/tests/e2e/live-openclaw-injection.spec.ts`
- Topic participation and content evaluation permissioning: TC-050/TC-060
  - `webapp/tests/e2e/live-topic-flow.spec.ts`

