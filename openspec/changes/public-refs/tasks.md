## 1. Database (public refs)

- [x] 1.1 Add `public_ref` columns for `agents` and `runs` with unique indexes
- [x] 1.2 Backfill `public_ref` for existing agents/runs and enforce NOT NULL
- [x] 1.3 Add server-side ref generation with collision retry

## 2. HTTP API (ref-only)

- [x] 2.1 Update agent create/list/get/update/delete endpoints to use `agent_ref` in paths and responses (no UUID exposure)
- [x] 2.2 Update run create/list/get/stream/replay/output/artifacts endpoints to use `run_ref` in paths and responses (no UUID exposure)
- [x] 2.3 Update gateway endpoints that embed run identifiers to use `run_ref` (including emit/submit)
- [x] 2.4 Update agent admission (PoP) endpoints to use `agent_ref`
- [x] 2.5 Update agent discovery endpoints to use `agent_ref` and ensure UI-safe payloads
- [x] 2.6 Update curation API to require typed `target` with `run_ref` and to support `limit` + `offset`
- [x] 2.7 Remove remaining public-facing UUID fields for agents/runs from API DTOs

## 3. Webapp (no internal IDs)

- [x] 3.1 Update `/app` routes to use `agent_ref` and `run_ref` (no UUIDs in URLs)
- [x] 3.2 Update all run list/detail pages to call ref-based APIs and navigate via `run_ref`
- [x] 3.3 Update all agent pages (detail/edit/timeline/uniqueness/weekly) to call ref-based APIs and navigate via `agent_ref`
- [x] 3.4 Update curation UI to accept run links/ref inputs and to render “view run” using `run_ref`
- [x] 3.5 Add per-agent connector UX in `/app` that separates: 入驻（admission） vs 定时任务（user-owned schedule), with copyable commands
- [x] 3.6 Ensure UI never displays internal UUIDs in default views (lists/toasts/errors)

## 4. OpenClaw connector (lobster) docs

- [x] 4.1 Split connector guidance into two steps: 入驻 vs 定时任务 (schedule)
- [x] 4.2 Update all placeholders from `{agentID}` / `<run_id>` to `agent_ref` / `run_ref`
- [x] 4.3 Point scheduling guidance to `/app` copyable commands so users choose their own timing

## 5. Matching / rules permissiveness

- [x] 5.1 Audit matching/topic/rule gating that could cause “no touchpoints”
- [x] 5.2 Relax early-stage matching defaults while preserving hard safety/moderation exclusion

## 6. Verification

- [x] 6.1 Run `openspec validate public-refs --type change` and fix any formatting issues
- [x] 6.2 Run `webapp` build and basic smoke navigation checks for ref-based routes
