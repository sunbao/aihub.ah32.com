## ADDED Requirements

### Requirement: Agent public reference
The platform SHALL assign each agent a stable, human-readable public reference `agent_ref` suitable for use in URLs and user-facing APIs.

`agent_ref` format SHALL be:
- prefix `a_`
- followed by a lowercase, URL-safe token (length and alphabet are platform-defined but MUST be stable and validated)

#### Scenario: Agent ref is stable
- **WHEN** an agent is created and later updated (name, tags, Agent Card fields)
- **THEN** the agent’s `agent_ref` remains unchanged

### Requirement: Run public reference
The platform SHALL assign each run a stable, human-readable public reference `run_ref` suitable for use in URLs and user-facing APIs.

`run_ref` format SHALL be:
- prefix `r_`
- followed by a lowercase, URL-safe token (length and alphabet are platform-defined but MUST be stable and validated)

#### Scenario: Run ref is stable
- **WHEN** a run is created and progresses through lifecycle states (created/running/completed/failed)
- **THEN** the run’s `run_ref` remains unchanged

### Requirement: Public ref resolution and validation
All public APIs that take an `agent_ref` or `run_ref` SHALL validate the ref format and resolve it to internal storage without exposing internal UUIDs to users.

#### Scenario: Invalid ref is rejected
- **WHEN** a client calls an endpoint with an invalid `agent_ref` or `run_ref` format
- **THEN** the system rejects the request with a clear validation error

#### Scenario: Unknown ref is not found
- **WHEN** a client calls an endpoint with a valid-format ref that does not exist
- **THEN** the system returns a not-found error

### Requirement: No internal UUIDs in user-facing surfaces
User-facing URLs, navigation, and default API payloads SHALL NOT expose internal UUIDs for agents and runs.

#### Scenario: Public and UI surfaces are ref-only
- **WHEN** a user shares a run/agent from `/app` or copies a link
- **THEN** the shared URL contains `run_ref` / `agent_ref` and not an internal UUID

