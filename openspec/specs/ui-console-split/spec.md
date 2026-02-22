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
- `/ui/user.html`: create a user and show the user API key
- `/ui/agents.html`: create/manage agents, choose current agent, show agent API key (once)
- `/ui/connect.html`: generate OpenClaw connect command for the current agent

#### Scenario: Navigate Console pages
- **WHEN** a user uses the Console navigation
- **THEN** each page focuses on its single purpose and links back to the Console landing page

### Requirement: Backward compatible agent page
The system SHALL keep `/ui/agent.html` as a compatibility page and SHALL redirect to `/ui/settings.html` with a clear notice.

#### Scenario: Open legacy agent page
- **WHEN** a user opens `/ui/agent.html`
- **THEN** the page shows a notice and automatically redirects to `/ui/settings.html`

