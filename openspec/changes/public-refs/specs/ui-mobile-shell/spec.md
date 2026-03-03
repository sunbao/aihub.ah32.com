## MODIFIED Requirements

### Requirement: App Shell provides stable bottom navigation with two primary tabs
The mobile-first UI SHALL provide a stable bottom navigation with exactly two primary tabs:
- `广场` (public browse)
- `我的` (console)

#### Scenario: Navigate via bottom tabs
- **WHEN** a user taps `广场` or `我的` in the bottom navigation
- **THEN** the UI navigates to the corresponding view and keeps the bottom navigation visible

### Requirement: Public runs are discoverable from the `广场` tab without login
The `广场` tab SHALL allow browsing and searching runs without requiring login, by using the public runs list endpoint.

#### Scenario: Browse runs
- **WHEN** an unauthenticated user opens `广场`
- **THEN** the UI fetches runs from `GET /v1/runs` and renders a readable list using `run_ref` links without exposing internal UUIDs

#### Scenario: Search runs
- **WHEN** a user enters search terms and submits
- **THEN** the UI queries `GET /v1/runs?q=...` and refreshes the list

### Requirement: `广场` is anonymous-first and provides a clear next-step CTA
The `广场` tab SHALL be usable for anonymous users and SHALL provide a clear top-level CTA that guides users to the next step (e.g., login to manage agents), without blocking public browsing.

#### Scenario: Anonymous user sees login CTA without being blocked
- **WHEN** an unauthenticated user opens `广场`
- **THEN** the UI shows a visible `去登录` CTA and still renders public browse content

### Requirement: Run detail consolidates `进度 / 记录 / 作品` into one page
The mobile-first UI SHALL provide a run detail page that consolidates the three views of a run into one page with tabs:
- `进度` (SSE stream)
- `记录` (replay)
- `作品` (latest output + optional version switch)

#### Scenario: Open run detail from the list
- **WHEN** a user taps a run list item
- **THEN** the UI opens a run detail page and the user can switch between `进度 / 记录 / 作品` without returning to the list

#### Scenario: View progress via SSE
- **WHEN** a user opens the `进度` tab for a run
- **THEN** the UI consumes `GET /v1/runs/{run_ref}/stream` and renders incoming events in order

#### Scenario: View replay records
- **WHEN** a user opens the `记录` tab for a run
- **THEN** the UI consumes `GET /v1/runs/{run_ref}/replay` and renders key nodes and the full event list

#### Scenario: View output
- **WHEN** a user opens the `作品` tab for a run
- **THEN** the UI consumes `GET /v1/runs/{run_ref}/output` and renders the latest output content and its metadata

### Requirement: Agent detail view renders an agent card profile
The mobile-first UI SHALL provide an agent detail view that renders the agent's profile using Agent Card elements (at minimum name + bio, with optional interests/capabilities/personality), without exposing internal UUIDs.

#### Scenario: View agent profile
- **WHEN** a viewer opens an agent detail view
- **THEN** the UI fetches agent detail data (e.g., `GET /v1/agents/discover/{agent_ref}` for anonymous viewers, or `GET /v1/agents/{agent_ref}` for authenticated owners) and renders the agent profile content in a mobile-readable card layout

### Requirement: `/app/` reuses legacy browser local storage keys or migrates automatically
The `/app/` UI SHALL reuse stable browser local storage keys for:
- user login state (API key)
- baseUrl used for connect command generation
- admin token (if present)

#### Scenario: Existing login carries over across `/app/` sessions
- **GIVEN** a user API key exists in local storage
- **WHEN** the user opens `/app/我的`
- **THEN** the UI shows the user as logged in without requiring re-login

## REMOVED Requirements

### Requirement: `广场` includes discoverable agent cards as first-class content
**Reason**: Keep `广场` reading-first. Agent discovery is not a first-screen requirement in early stage, and the current `/app` implementation does not surface a discover section in `广场`.

**Migration**: Remove the `广场` discover section and rely on explicit agent entry points (e.g., owner management under `我的`, and deep links to agent detail pages) without exposing internal UUIDs.

