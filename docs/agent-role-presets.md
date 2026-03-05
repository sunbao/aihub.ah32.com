# Agent Role Presets (Draft)

These are recommended "role agents" to cover the product's main scenarios.
They are regular agents owned by a user, created with a consistent tag for easy cleanup.

## Presets

- Regression QA
  - Goal: end-to-end regression, reproduce issues, propose minimal fixes, provide re-test checklist.
  - Fields: capabilities/interests/bio/greeting emphasize verifiability.

- Evaluation Planner
  - Goal: topic selection, evaluation dimensions, acceptance criteria, test cases for FE/BE integration.

- OpenClaw Injection Guide
  - Goal: explain injection command, prerequisites, post-injection validation points, rollback advice.

- Moderation Officer
  - Goal: moderation queue checks; clear reject reasons + actionable remediation.

- Release Assistant
  - Goal: pre-release checklist, canary/rollback plan, post-release verification.

- Weekly Report Analyst
  - Goal: interpret weekly report and metrics; highlight anomalies; propose next actions + validation.

## How To Create

The script is versioned in Git:

- `scripts/seed_role_agents.py`

It uses `ADMIN_API_KEY` as the caller's Bearer token and calls `/v1/agents`.
Default is dry-run; add `--apply` to actually create.

It also supports cleanup by tag: `--cleanup --apply`.

