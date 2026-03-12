---
name: aihub-qa-loop
description: End-to-end QA loop for AIHub against a remote docker host (192.168.1.154): run UI (Playwright) + API (smoke*.sh) suites, then iterate fix -> commit -> remote deploy -> re-test, and record PASS in OpenSpec verification.md.
---

# AIHub QA Loop (Remote Docker Host)

## Inputs (local env)

- `PLAYWRIGHT_BASE_URL`: example `http://192.168.1.154:8080`
- `ADMIN_API_KEY`: an `is_admin=true` user API key (used as `Authorization: Bearer <key>`)
- `AIHUB_SSH_PASSWORD`: SSH password for the docker host (root)

## System Run/Deploy Flow (must follow)

1. Make code changes locally (Windows).
2. `git commit` + `git push` to the repo.
3. On `192.168.1.154` (docker host): `git pull` then `docker compose build/up` to rebuild + restart.
4. Verify API and worker are healthy, then run smoke + UI E2E.

## Suite Order (recommended)

1. Deploy + API smoke on docker host (requires `ADMIN_API_KEY`)
- `python scripts/remote/aihub_154_deploy_and_smoke.py --host 192.168.1.154 --user root --base-url http://192.168.1.154:8080`

2. UI E2E (Playwright, live against `PLAYWRIGHT_BASE_URL`)
- `npm -C webapp run test:e2e:openspec`

## Scenario Flows (what "按流程测试" means)

These are separate, ordered business flows. E2E coverage should map to these explicitly.

1. Agent creation (includes pre-review evaluation)
- Create agent (card fields set) -> reach "提交前测评" step -> select a real topic -> start evaluation -> inspect snapshot -> delete evaluation and related run.

2. Agent participates in a topic
- Create an invite topic (allowlist includes the agent) -> write a topic message via gateway -> verify it appears in the public topic thread -> cleanup the topic.

3. Agent evaluates content in a topic
- For a mode that supports vote-style requests (example: `poetry_duel` with `rules.judge_mode` containing `vote`) -> write a `vote` request via gateway -> cleanup the topic.

4. Square homepage shows latest activity
- Create a public run and emit a key-node event -> verify it appears on `/app/` "Run activity" feed.

## Iteration Loop (manual trigger, auto sequencing)

- Run suites in the order above.
- On first failure:
- Capture the failing route/API and error output (do not ignore).
- Fix root cause in code.
- Commit + push to Git.
- Re-run deploy + smoke on 154.
- Re-run the failing suite, then continue with the next suite.

## Data Hygiene

- Every test that creates agents/runs/evaluations MUST delete them during cleanup.
- If a failure prevents cleanup, immediately delete the created data before proceeding.

## OpenSpec PASS Marker

- After all suites pass, append a new `[PASS]` entry with timestamp, environment, base URL, commit, and suites to each relevant `openspec/changes/*/verification.md`.
