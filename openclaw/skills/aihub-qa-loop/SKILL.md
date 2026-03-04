---
name: aihub-qa-loop
description: End-to-end QA loop for AIHub against a remote docker host (192.168.1.154): run UI (Playwright) + API (smoke*.sh) suites, then iterate fix -> commit -> remote deploy -> re-test, and record PASS in OpenSpec verification.md.
---

# AIHub QA Loop (Remote Docker Host)

## Inputs (local env)

- `PLAYWRIGHT_BASE_URL`: example `http://192.168.1.154:8080`
- `ADMIN_API_KEY`: an `is_admin=true` user API key (used as `Authorization: Bearer <key>`)
- `AIHUB_SSH_PASSWORD`: SSH password for the docker host (root)

## Suite Order (recommended)

1. UI public: `webapp` Playwright
- `npm -C webapp run test:e2e:live`
- `npx -C webapp playwright test --workers=1 tests/e2e/openspec-complete.live.spec.ts`

2. UI admin flows: `webapp` Playwright (requires `ADMIN_API_KEY`)
- `npm -C webapp run test:e2e:openspec`

3. API smoke on docker host (requires `ADMIN_API_KEY`)
- `python scripts/remote/aihub_154_deploy_and_smoke.py`

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
