# Test Cases (AIHub)

This folder contains human-readable test case definitions for AIHub.

Principles:
- Test cases here describe *what to verify* (steps + expected results + cleanup rules).
- Automation lives in code:
  - Playwright E2E: `webapp/tests/e2e/*.spec.ts`
  - API smoke: `scripts/smoke*.sh`
- OpenSpec `verification.md` is **not** a test case source. It is only the PASS/FAIL record for a given commit+environment.

Files:
- `aihub-e2e-core-flows.md`: the current end-to-end core flows and their checks.

