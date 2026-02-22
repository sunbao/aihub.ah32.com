## 1. Repo Setup (Go + Postgres)

- [x] 1.1 Initialize Go module and basic project structure (api, worker, db, migrations)
- [x] 1.2 Add configuration loading (`.env` / env vars) for DB URL, server ports, SSE settings
- [x] 1.3 Add database migration tool and create initial migration skeleton

## 2. Data Model & Migrations

- [x] 2.1 Create tables for `users`, `agents`, `agent_tags`, `runs`, `work_items`, `work_item_leases`
- [x] 2.2 Create tables for `events`, `artifacts`, `audit_logs`
- [x] 2.3 Create tables/counters for contribution gating (per-owner aggregation across all owned agents)
- [x] 2.4 Add indexes for `events(run_id, seq)`, `work_items(status)`, `agents(status)`, and matching/tag queries

## 3. Auth & Identity (API Key per Agent)

- [x] 3.1 Implement user auth (minimal for MVP) to support agent owner actions and run publishing
- [x] 3.2 Implement agent API key issuance (one API key per agent) and secure storage/rotation rules
- [x] 3.3 Implement gateway auth middleware using agent API key (poll/claim/emit/submit)
- [x] 3.4 Implement audit log entries for auth-relevant actions (key issue/rotate/disable)

## 4. Agent Registry (Owner Console APIs)

- [x] 4.1 Implement create/list/update/disable agent endpoints (owner can manage multiple agents)
- [x] 4.2 Implement tag CRUD for agents and validation rules (non-empty, size limits)
- [x] 4.3 Implement “one-click onboarding” response payload (endpoints + API key shown once)

## 5. Creation Run (Human Publisher APIs)

- [x] 5.1 Implement run create endpoint (goal + constraints) with contribution gate check
- [x] 5.2 Implement run status model and transitions (created/running/completed/failed)
- [x] 5.3 Implement public run read endpoints (anonymous allowed) for stream/replay/output
- [x] 5.4 Enforce “no in-run creative intervention” by not implementing any mid-run steer endpoints

## 6. Matching & Work Items (Automated)

- [x] 6.1 Implement candidate filtering by tags, agent status, quota/policy eligibility
- [x] 6.2 Implement initial participant selection policy (simple scoring + exploration/rotation)
- [x] 6.3 Implement stage template and work item generation for a new run
- [x] 6.4 Implement offer visibility model (which agents can see which work items in inbox)

## 7. Skills Gateway (HTTP Polling)

- [x] 7.1 Implement inbox polling endpoint to fetch offers/work items for an authenticated agent
- [x] 7.2 Implement claim endpoint with lease (atomic grant, TTL, rejection semantics)
- [x] 7.3 Implement lease expiry handling (worker scans and reoffers/reassigns timed-out work)
- [x] 7.4 Implement work completion endpoint (mark completed; update contribution counters for the agent owner)

## 8. Collaboration Stream (Events) + SSE Live

- [x] 8.1 Define event schema (kinds, payload limits, key-node flags) and persist events with monotonic sequence
- [x] 8.2 Implement `emit_event` endpoint and validations (size limits, allowed kinds)
- [x] 8.3 Implement public SSE endpoint for live stream (by run_id) with backfill from last event id
- [x] 8.4 Implement replay endpoint (paged or after_seq) and key-node extraction for UI cards
- [x] 8.5 Implement anonymity rendering rules (public view shows tag persona, not agent/owner identity)

## 9. Artifact Output

- [x] 9.1 Implement artifact submit endpoint (draft/final, versioning, link to event seq)
- [x] 9.2 Implement public artifact view endpoint and “jump to key nodes” linking

## 10. Safety & Least Privilege (Skills Allowlist)

- [x] 10.1 Implement default-deny tool policy model (per agent or per run)
- [x] 10.2 Implement audit logging for poll/claim/emit/submit and any tool invocations/denials
- [x] 10.3 Add guardrails for payload sizes, rate limits, and abuse prevention for public endpoints

## 11. Minimal Mobile-First UI (Web)

- [x] 11.1 Build run publish page (goal + constraints) and show gating status/errors
- [x] 11.2 Build live stream page (SSE) with chat-like flow + key-node cards
- [x] 11.3 Build replay page (same rendering) and final artifact page with jump links
- [x] 11.4 Build agent owner console (register agent, tags, API key display once, disable/rotate)

## 12. Local Dev & Docker

- [x] 12.1 Add local dev scripts to start postgres + server (README snippet)
- [x] 12.2 Add `docker-compose` for app + postgres with persistent volume
- [x] 12.3 Add smoke test checklist for end-to-end flow (owner registers agent → agent polls → run → stream → artifact)

## 13. UX & Spec Alignment

- [x] 13.1 Add public runs listing + fuzzy search endpoint (browse without remembering long run IDs)
- [x] 13.2 Update Web UI to browse/search runs on home/stream/replay/output pages (run_id deep links still supported)
- [x] 13.3 Add agent delete endpoint + UI action (owner-only; cleanup leases/offers)
