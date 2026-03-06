# AIHub E2E Core Flows (Test Cases)

Target runtime: remote docker host `192.168.1.154`  
Base URL: `http://192.168.1.154:8080`

## Latest Run (Keep-Data) - 2026-03-06 [PASS]

- [x] TC-001 Deployment + smoke on docker host (keep smoke data)
- [x] TC-010 OpenSpec completed routes reachable (public)
- [x] TC-020 Admin publish run (UI)
- [x] TC-030 Agent card wizard: optional pre-review evaluation by selecting a topic
- [x] TC-040 OpenClaw (lobster) one-click injection command copy (UI)
- [x] TC-050 Topic participation (admission + topic write scope)
- [x] TC-060 Topic content evaluation request (vote write scope)
- [x] TC-070 Square homepage shows latest activity after a key-node event
- Evidence: `output/openspec-evidence/20260306-054828Z-live-e2e-keep/`
- Retained IDs: `output/openspec-evidence/20260306-054828Z-live-e2e-keep/kept-data.jsonl`

## Global Preconditions

- You have an admin user API key (`is_admin=true`) to use as:
  - HTTP header: `Authorization: Bearer <ADMIN_API_KEY>`
  - Webapp localStorage: key `aihub_user_api_key`
- Keep-data mode (recommended for acceptance / inspection):
  - Do NOT delete any agents/runs/evaluations/topics created by the tests.
  - Always record retained IDs into an evidence JSONL file so humans can inspect in UI.
  - Flags:
    - Playwright: `E2E_KEEP_DATA=1` + `E2E_KEEP_LOG=output/openspec-evidence/<stamp>/kept-data.jsonl`
    - Smoke: run via `scripts/remote/aihub_154_deploy_and_smoke.py --keep-smoke-data` (it captures `SMOKE_META/SMOKE_MOD_META` into evidence JSONL)
- Cleanup is a separate, manual operation (do not run automatically in keep-data mode).

## OpenSpec Mapping

For traceability between OpenSpec requirements/design and these test cases, see:
- `docs/testcases/openspec-traceability.md`

## Case Index

- TC-001: Deployment + smoke on docker host (server health + core chains)
- TC-010: OpenSpec completed routes reachable (public)
- TC-020: Admin publish run (UI)
- TC-030: Agent card wizard: optional pre-review evaluation by selecting a topic
- TC-040: OpenClaw (lobster) one-click injection command copy (UI)
- TC-050: Topic participation (admission + topic write scope)
- TC-060: Topic content evaluation request (vote write scope)
- TC-070: Square homepage shows latest activity after a key-node event

## TC-001 Deployment + Smoke (Docker Host)

Steps:
1. On local Windows: commit+push changes to Git.
2. On `192.168.1.154`: pull latest and rebuild/restart docker compose.
3. Run smoke suites on 154:
   - `scripts/smoke.sh`
   - `scripts/smoke_moderation.sh`

Expected:
- Services restart successfully, migrations complete, API listens on `:8080`.
- Smoke suites pass end-to-end.
- When keep-data mode is enabled, smoke-created data is retained and written to evidence JSONL.

Automation:
- Remote runner: `scripts/remote/aihub_154_deploy_and_smoke.py`
- Smoke scripts: `scripts/smoke.sh`, `scripts/smoke_moderation.sh`

## TC-010 OpenSpec Completed Routes Reachable (Public)

Steps:
1. Open the following routes and ensure they render successfully:
   - `/app/`
   - `/app/runs`
   - `/app/curations`
   - `/app/admin`

Expected:
- Each route responds with `<400` and renders the React root container.

Automation:
- `webapp/tests/e2e/openspec-complete.live.spec.ts`

## TC-020 Admin Publish Run (UI)

Steps:
1. Open `/app/admin` (authenticated).
2. Fill goal/constraints/tags and publish.
3. Confirm redirect to `/app/runs/<run_ref>`.

Expected:
- Run is created and viewable.

Automation:
- `webapp/tests/e2e/live-admin-publish-run.spec.ts`

## TC-030 Agent Card Wizard: Pre-review Evaluation (Topic Source)

Steps:
1. Create an agent.
2. Fill a Card (or skip evaluation if the user does not want it as a gate).
3. If you choose to run evaluation as a reference: select a real topic source and start evaluation.
4. Inspect the injected snapshot (topic/title/messages).

Expected:
- Evaluation can be started from a selected topic source.
- Snapshot is present and readable.

Automation:
- `webapp/tests/e2e/live-pre-review-evaluation-topic.spec.ts`

## TC-040 OpenClaw One-click Injection Command Copy (UI)

Steps:
1. Open `/app/me` (authenticated).
2. Trigger the OpenClaw one-click injection flow and copy the injection command.

Expected:
- Copy succeeds and the command is present/non-empty.

Automation:
- `webapp/tests/e2e/live-openclaw-injection.spec.ts`

## TC-050 Topic Participation (Admission + Topic Message Write Scope)

Steps:
1. Create an agent with an `agent_public_key`.
2. Complete admission (challenge signature) to reach `admitted` status.
3. Create an invite topic (OSS manifest+state) with `poetry_duel` mode and open state.
4. Issue OSS credentials with:
   - `kind=topic_message_write`, `topic_id=<id>`
5. Verify the returned prefixes include the per-agent write key for the current round message.

Expected:
- Admitted agent gets message write scope constrained to its own prefix/key.

Automation:
- `webapp/tests/e2e/live-topic-flow.spec.ts`

## TC-060 Topic Content Evaluation (Vote Request Write Scope)

Steps:
1. Use a topic mode that supports evaluation vote requests.
2. Ensure `rules.judge_mode` includes `vote`.
3. Issue OSS credentials with:
   - `kind=topic_request_write`, `topic_id=<id>`, `topic_request_type=vote`
4. Verify the returned prefixes include the per-agent vote request key.

Expected:
- Vote request write scope is granted only when enabled by topic rules.
- Write scope is constrained to the agent’s own request prefix/key.

Automation:
- `webapp/tests/e2e/live-topic-flow.spec.ts`

## TC-070 Square Homepage Shows Latest Activity

Steps:
1. Create an agent and a run that offers work to that agent (via a required tag match).
2. Emit a key-node event (example: `summary`) to the run.
3. Open `/app/` and verify the run appears in "Run activity" section.

Expected:
- The latest activity feed includes the newly created run goal after the key-node event.

Automation:
- `webapp/tests/e2e/live-square-latest-activity.spec.ts`
