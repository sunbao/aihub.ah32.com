# Project rules

## 1) File size

- Any source file over **2000 lines** must be split into smaller, focused files.

## 2) Error handling

- Do **not** swallow errors (no silent ignores).
- Avoid nested error handling; prefer early returns.
- Every error must be logged.
- For user-facing failures, return a clear error response/message and notify the user when appropriate.
- No “fallback / compatibility with old versions” mindset: the product runs as **latest-only**; deprecated behaviors MUST be removed (no shims/fallbacks).

## 3) UI integration (delete or integrate)

- Keep a **single** product UI for console/management: `/app` (the webapp).
- Do **not** add new console/management features to `/ui`.
- No “compatibility/downgrade/fallback/回退” mindset: deprecated things MUST be removed; needed things MUST be integrated into `/app`.
- No parallel implementations: if a feature is needed, **integrate** it into `/app`; if not needed, **delete** it.
- When integrating from `/ui` -> `/app`, remove the `/ui` page/route/assets in the same change and update internal links/docs accordingly (no shims/fallbacks).
- UI MUST NOT surface internal IDs/UUIDs (e.g. `persona_xxx_v1`, raw UUIDs) to end users; always show a human-readable label.

## 4) Agent UX (no “current agent”)

- Do **not** introduce a global “current agent”/“set as current” concept in console UX.
- Agent-bound actions must be **per-agent** (explicit `agent_id`), and should work without extra “select current agent” steps.

## 5) Production data hygiene (delete test data)

- Treat this environment as **real production**.
- Any **test/demo data** created during development, debugging, or validation must be **deleted/rolled back** after use.

## 6) Execution transparency (announce before running)

- When the user grants **highest/full access** permissions, before executing any instruction you must **first tell the user exactly what you are going to run/change** (commands, files, and intended effects), then execute.
- This enables the user to decide whether to **stop/adjust** early; do **not** wait until after execution to summarize the changes.
