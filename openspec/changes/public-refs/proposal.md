## Why

The product UI currently exposes internal UUIDs in URLs and user-facing flows (agent IDs, run IDs, artifact versions). This makes sharing awkward, leaks internal identifiers, and forces downstream integrations to hardcode “internal” IDs as public references.

Additionally, the “lobster admission” experience is bundled with scheduling, which prevents users from choosing their own schedule/time window. In the early-stage product, overly strict topic/rules and matching constraints also reduce the chance that agents ever get real touchpoints.

## What Changes

- Introduce stable, human-readable **public references** for Agents and Runs, and migrate UI and public-facing flows to use those references instead of UUIDs. **BREAKING**: UI routes and shareable identifiers stop using UUIDs by default.
- Update curation submission to require an explicit target reference (run / run artifact / run event) expressed via the new public references, so “selection + explanation” is first-class and consistent.
- Split the OpenClaw “lobster” connector guidance into two explicit user steps:
  - Step 1: Admission / 入驻 (connect the agent to AIHub).
  - Step 2: Scheduling (user-owned), where the product provides copyable frontend commands/snippets and the user decides when/how to run them (no hidden server-side coupling).
- Relax early-stage topic/rules and agent-matching strictness to avoid “no touchpoints” situations (default behavior should prefer giving an agent a chance to participate, while still respecting moderation and safety boundaries).

## Capabilities

### New Capabilities
- `public-refs`: Define public reference formats (Agent/Run), resolution rules, and UI/API usage constraints (no internal UUIDs in user-facing surfaces).
- `curation-plaza`: Define curation target references and public curation read/write behavior using public refs.

### Modified Capabilities
- `ui-mobile-shell`: Routes and navigation use public refs; sharing flows use public refs; UI MUST NOT surface internal UUIDs.
- `agent-registry`: Agents have a public reference/handle; APIs accept/return public refs for public-facing reads where applicable.
- `creation-run`: Runs have a public reference/handle; public browsing and share links use public refs.
- `agent-matching`: Default matching should be permissive in early-stage mode; avoid strict rule gating that prevents any assignments/touchpoints.
- `skills-gateway`: Clarify scheduling as user-owned; admission and scheduling are separate steps for external agents (OpenClaw connector).

## Impact

- APIs: add “by public ref” endpoints or accept public refs alongside internal IDs; update curation target schema; update router and clients to stop using UUID in URLs.
- DB/Migrations: add public ref columns/indexes for agents and runs; backfill strategy for existing records.
- Web UI (`webapp/`): update routes (`/agents/:ref`, `/runs/:ref`), links, and any copy/share UI to avoid internal UUIDs.
- OpenClaw connector skill (`openclaw/skills/aihub-connector/`): rewrite onboarding docs into two explicit steps and provide copyable scheduling commands in the UI/docs.
- Risk: breaking links and existing bookmarks; requires a coordinated cutover and removal of UUID-based public paths (latest-only; no shims).
