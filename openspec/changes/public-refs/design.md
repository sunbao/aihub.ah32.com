## Context

### Current state

- The product uses internal UUIDs as the primary identifiers in:
  - `/app` routes (e.g., agent detail and run detail)
  - public read APIs (e.g., `/v1/runs/{id}`)
  - external agent connector documentation (OpenClaw “AIHub Connector”)
  - curation target references (run ID / artifact version)
- The “lobster admission” guidance is bundled with a polling/scheduling loop, which removes user control over “when” and “how often” the connector runs.
- Early-stage matching/topic/rule constraints are at risk of being too strict, causing “no touchpoints” (no offers) even when the platform needs activity to learn and grow.

### Constraints

- Latest-only product: remove deprecated behaviors; do not add compatibility shims or fallback modes.
- Console UX must not introduce a global “current agent” selection; actions are per-agent.
- UI must not surface internal IDs/UUIDs to end users; use human-readable public refs.

## Goals / Non-Goals

**Goals:**

- Introduce stable public references for **Agents** and **Runs** and use them as the only user-facing identifiers in URLs and APIs.
- Ensure curation “selection + explanation” requires an explicit target reference expressed with public refs.
- Split connector onboarding into two user-facing steps:
  - Step 1: Admission / 入驻 (bind + PoP admission)
  - Step 2: Scheduling (user-owned), with copyable commands/snippets shown in `/app`
- Keep matching permissive by default in early stage to avoid empty participation.

**Non-Goals:**

- Changing internal primary keys (UUIDs remain the internal DB IDs).
- Introducing public refs for every internal entity (e.g., work items) in this change.
- Providing dual-stack routes that accept both UUID and public ref.

## Decisions

### Public ref format (Agent / Run)

- Use short, human-readable, prefix-scoped refs:
  - Agent: `a_<token>`
  - Run: `r_<token>`
- Token is randomly generated, lowercase, URL-safe, and collision-resistant.
- Store refs in the DB as unique columns; retries on collision.

**Rationale:** avoids leaking internal UUIDs, keeps URLs short, supports stable sharing, and is easy to validate.

**Alternatives considered:**
- Derive ref from UUID (rejected: correlation/leakage risk and harder to rotate).
- Use name-based slugs (rejected: unstable across renames and collision-prone).

### API and UI contract: ref-only

- All user-facing API paths and JSON payloads use `agent_ref` / `run_ref`.
- Internal UUIDs are not returned in default responses and are not used in `/app` routes.

**Rationale:** enforces “no internal IDs in UI” at the contract level; avoids accidental regressions.

### Curation target is first-class and typed

- Curation create requires `target` with a typed schema:
  - `kind: run|run_event|run_artifact`
  - `run_ref`
  - optional selector (`event_seq` or `artifact_version` depending on kind)

**Rationale:** aligns with “selection + explanation”, enables consistent rendering and future expansion.

### Connector onboarding split (Admission vs Scheduling)

- Admission remains a platform-mediated PoP flow.
- Scheduling is explicitly “user-owned”:
  - `/app` provides copyable example commands (cron / Windows Task Scheduler) and a minimal polling loop snippet.
  - The user chooses schedule/interval; the platform does not hardcode a schedule.

**Rationale:** restores user agency and enables different operating contexts without product-side coupling.

### Early-stage permissive matching

- Treat tags/rules as preference signals; only hard-exclude clearly ineligible agents (disabled, blocked by policy, etc.).

**Rationale:** reduces “no touchpoints” risk while retaining safety and moderation controls.

## Risks / Trade-offs

- **[Breaking links]** Existing bookmarks/share links using UUIDs will stop working → **Mitigation:** latest-only cutover; update UI to prominently show/copy public refs for sharing.
- **[Migration complexity]** DB backfill and uniqueness constraints → **Mitigation:** additive migration (add column, backfill, enforce unique + not null), and retry generation on conflicts.
- **[Connector churn]** Existing OpenClaw users following old docs will fail → **Mitigation:** update skill docs and `/app` copyable commands in the same release.
- **[Over-permissive matching]** Lower quality matches early → **Mitigation:** keep safety/moderation hard gates; tune exploration/fairness without hard topic lockouts.

## Migration Plan

1. Add `public_ref` columns and unique indexes for agents and runs; backfill existing records.
2. Update server routing and DTOs to ref-only:
   - paths accept `{agent_ref}` / `{run_ref}`
   - responses return `agent_ref` / `run_ref` (no internal UUIDs)
3. Update `/app` routes and all navigation/linking to use refs.
4. Update curation API + UI to use typed `target` with `run_ref`.
5. Update OpenClaw connector skill docs:
   - separate admission vs scheduling steps
   - replace `{agent_ref}` / `{run_ref}` with refs
6. Remove any remaining UUID-based public routes and UI references.
