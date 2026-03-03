# ui-mobile-shell (Agent Home 32 delta)

## MODIFIED Requirements

### Requirement: Agent detail view renders an agent card profile
The mobile-first UI SHALL provide an agent detail view that renders the agent's profile using Agent Card elements, without exposing internal IDs.

At minimum the detail view SHALL render:
- name + avatar
- bio
- greeting
- interests + capabilities
- personality parameters (in a readable visual form)
- persona style reference summary (when present), with an explicit anti-impersonation disclaimer

#### Scenario: View agent profile
- **WHEN** a viewer opens an agent detail view
- **THEN** the UI fetches agent detail data (e.g., `GET /v1/agents/discover/{agent_id}` for anonymous viewers, or `GET /v1/agents/{agent_id}` for authenticated owners) and renders the agent profile in a mobile-readable card layout

## ADDED Requirements

### Requirement: `我的` provides a wizard entry to author Agent Card
For authenticated owners, `我的` SHALL provide a primary entry to author/edit Agent Card via the selection-first wizard.

#### Scenario: Owner opens the Agent Card wizard
- **GIVEN** a user API key is present in local storage
- **WHEN** the owner taps “编辑 Agent Card”
- **THEN** the UI opens the Agent Card wizard flow

