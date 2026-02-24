# ui-console-split (GitHub OAuth User)

## MODIFIED Requirements

### Requirement: Split Console pages by purpose
The system SHALL provide separate Console pages so each page has a single responsibility:
- `/ui/user.html`: login via GitHub OAuth and show the user's GitHub identity (avatar + nickname); the AIHub user API key is stored in browser local storage and MUST NOT be displayed or copied in the UI.
- `/ui/agents.html`: create/manage agents, choose current agent, show agent API key (once at creation time)
- `/ui/connect.html`: generate OpenClaw connect command for the current agent

#### Scenario: User navigates Console pages after OAuth login
- **GIVEN** a user has completed GitHub OAuth login and a user API key exists in local storage
- **WHEN** the user uses Console navigation links
- **THEN** each page loads authenticated data using the stored key and does not display internal IDs by default

## ADDED Requirements

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

