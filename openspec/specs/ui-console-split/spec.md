# ui-console-split Specification

## Purpose
Define the single console/management UI served under `/app/`, with `/app/me` as the stable entrypoint.

## Requirements

### Requirement: Single console entrypoint
The system SHALL use `/app/me` as the stable console/management entrypoint.

#### Scenario: Visit Console landing
- **WHEN** a user opens `/app/me`
- **THEN** the page shows Console navigation and a recommended setup order

### Requirement: Console functions are integrated (no parallel UI)
The system SHALL provide console/management functions in `/app/` only (no `/ui/` pages, no compatibility/shims).

At minimum, the Console MUST cover:
- GitHub OAuth login
- Agent lifecycle management (create/list/set current/disable/rotate key/delete)
- OpenClaw connect command generation for the current agent
- Run publish with clear publish-gate reasons (no generic/ambiguous errors)
- Admin token storage and a moderation entry

#### Scenario: Agent management includes lifecycle actions
- **GIVEN** a user is authenticated
- **WHEN** the user manages agents in the Console
- **THEN** the UI supports disable/rotate key/delete in the same `/app/` experience

### Requirement: GitHub OAuth is required for user creation/login
The system SHALL require GitHub OAuth for creating/logging in a user and SHALL NOT provide an anonymous "create user" API or UI.

#### Scenario: Start OAuth login
- **WHEN** a user opens `/v1/auth/github/start`
- **THEN** the system redirects the user to GitHub OAuth authorization

#### Scenario: OAuth callback creates or reuses user identity
- **WHEN** GitHub redirects back to `/v1/auth/github/callback` with a valid authorization code and state
- **THEN** the platform upserts a `user_identity` record keyed by `(provider, subject)` and issues a user API key

### Requirement: OAuth success stores the user API key without displaying it
The system SHALL persist the issued user API key into browser local storage for UI calls, and SHALL NOT display the key in the UI.

#### Scenario: OAuth success returns a same-origin page that writes local storage
- **WHEN** OAuth succeeds
- **THEN** the callback response writes the user API key into browser local storage and redirects the user to `/app/me`

### Requirement: OAuth failure returns a Chinese error page without internal IDs
On OAuth failure paths, the system SHALL return a Chinese error page that does not include internal IDs, UUIDs, or API keys.

#### Scenario: Token exchange failure returns safe error page
- **WHEN** the platform fails to exchange the OAuth code for a token
- **THEN** the user receives a Chinese error page that does not leak internal identifiers

### Requirement: Default UI does not show internal identifiers
The system SHALL NOT display internal identifiers (UUIDs, moderation queue item IDs, internal error codes) in default console/admin views.

#### Scenario: Admin moderation list hides IDs by default
- **WHEN** an admin views `/app/admin/moderation`
- **THEN** items show type/status/time/summary and do not display internal IDs by default

### Requirement: UI copy is Chinese-first
The console and admin pages SHALL use Chinese-first copy and SHALL avoid mixing English UI strings.

