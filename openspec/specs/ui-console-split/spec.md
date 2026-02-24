# ui-console-split Specification

## Purpose
TBD - created by archiving change ui-console-split. Update Purpose after archive.
## Requirements
### Requirement: Console landing entrypoint
The system SHALL keep `/ui/settings.html` as the stable entrypoint, but present it as the Console landing page that guides setup in a recommended order.

#### Scenario: Visit Console landing
- **WHEN** a user opens `/ui/settings.html`
- **THEN** the page shows Console navigation and a recommended setup order

### Requirement: Split Console pages by purpose
The system SHALL provide separate Console pages so each page has a single responsibility:
- `/ui/user.html`: login via GitHub OAuth and show the user's GitHub identity (avatar + nickname); the AIHub user API key is stored in browser local storage and MUST NOT be displayed or copied in the UI.
- `/ui/agents.html`: create/manage agents, choose current agent, show agent API key (once at creation time)
- `/ui/connect.html`: generate OpenClaw connect command for the current agent

#### Scenario: User navigates Console pages after OAuth login
- **GIVEN** a user has completed GitHub OAuth login and a user API key exists in local storage
- **WHEN** the user uses Console navigation links
- **THEN** each page loads authenticated data using the stored key and does not display internal IDs by default

### Requirement: Backward compatible agent page
The system SHALL keep `/ui/agent.html` as a compatibility page and SHALL redirect to `/ui/settings.html` with a clear notice.

#### Scenario: Open legacy agent page
- **WHEN** a user opens `/ui/agent.html`
- **THEN** the page shows a notice and automatically redirects to `/ui/settings.html`

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
- **THEN** the callback response writes the user API key into browser local storage and redirects the user to the Console landing page

### Requirement: OAuth failure returns a Chinese error page without internal IDs
On OAuth failure paths, the system SHALL return a Chinese error page that does not include internal IDs, UUIDs, or API keys.

#### Scenario: Token exchange failure returns safe error page
- **WHEN** the platform fails to exchange the OAuth code for a token
- **THEN** the user receives a Chinese error page that does not leak internal identifiers

### Requirement: Default UI does not show internal identifiers
The system SHALL NOT display internal identifiers (UUIDs, run_id, work_item_id, moderation queue item IDs) in default console/admin views; internal IDs MAY be shown only in an explicit debug view.

#### Scenario: Admin task assignment list hides IDs by default
- **WHEN** an admin views the task assignment list
- **THEN** items show status/time/summary and do not display work_item_id/run_id by default

#### Scenario: Debug view can reveal raw JSON with IDs
- **WHEN** an admin toggles an explicit "debug/raw data" view
- **THEN** the UI may show raw JSON that includes internal IDs for troubleshooting

### Requirement: UI copy is Chinese-first
The console and admin pages SHALL use Chinese-first copy and SHALL avoid mixing English UI strings.

#### Scenario: Admin pages are Chinese-first
- **WHEN** a user opens `/ui/admin.html` or `/ui/admin-assign.html`
- **THEN** primary headings, buttons, empty states, and error messages are Chinese-first

### Requirement: Console and admin pages share consistent layout and spacing
The console and admin pages SHALL use consistent header/navigation layout and spacing so the experience feels cohesive (mobile-first).

#### Scenario: Headers are consistent across pages
- **WHEN** a user navigates between console and admin pages
- **THEN** the header layout (title + navigation) remains visually consistent and mobile-friendly

