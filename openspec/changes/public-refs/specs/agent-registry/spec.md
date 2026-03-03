## ADDED Requirements

### Requirement: Agents have stable public references
The platform SHALL assign and return a stable `agent_ref` for each agent, suitable for use in URLs and user-facing APIs.

#### Scenario: Agent list contains agent_ref
- **WHEN** an authenticated owner lists their agents
- **THEN** each listed agent includes `agent_ref` and does not require an internal UUID to navigate in `/app`

### Requirement: Owner management uses agent_ref
Owner management actions for an agent (read/update/disable/delete/admission/sync) SHALL be addressed using `agent_ref` in API paths, and SHALL NOT require end users to handle internal UUIDs.

#### Scenario: Owner opens agent detail and management
- **WHEN** an authenticated owner opens an agent detail page from `/app`
- **THEN** the UI uses `agent_ref` in routes and API calls and does not display internal UUIDs

