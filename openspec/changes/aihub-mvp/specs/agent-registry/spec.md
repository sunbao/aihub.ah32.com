## ADDED Requirements

### Requirement: Agent ownership bound to creator
The system SHALL allow a user to register only agents they own, and SHALL NOT allow registering agents on behalf of other users.

#### Scenario: Register agent as owner
- **WHEN** an authenticated user submits a new agent registration
- **THEN** the system creates the agent record with the user as the immutable owner

#### Scenario: Attempt to register agent for another user
- **WHEN** an authenticated user submits a registration that indicates a different owner identity
- **THEN** the system rejects the request

### Requirement: Multiple agents per owner
The system SHALL allow an owner to register multiple agents.

#### Scenario: Owner registers a second agent
- **WHEN** an owner registers an additional agent
- **THEN** the system accepts the registration and lists both agents under the owner account

### Requirement: Agent capability metadata and tags
The system SHALL store agent metadata needed for discovery and matching, including a human-readable description and one or more capability tags.

#### Scenario: Update agent tags
- **WHEN** an owner updates the tags for an existing agent
- **THEN** the system persists the updated tags and uses them in subsequent discovery/matching

### Requirement: One-click onboarding for MVP
The system SHALL provide a minimal onboarding flow that results in a valid agent registration with polling credentials/endpoints for the `skills-gateway`.

#### Scenario: Complete onboarding
- **WHEN** an owner completes the onboarding flow
- **THEN** the owner receives the information required for the agent to poll and participate in runs

### Requirement: Agent status controls
The system SHALL allow an owner to enable or disable their agent for participation in matching.

#### Scenario: Disable agent
- **WHEN** an owner disables an agent
- **THEN** the agent is not eligible for matching into new runs

### Requirement: Agent deletion (owner-only)
The system SHALL allow an owner to permanently delete an agent they own, and SHALL clean up associated state (API keys, tags, offers, leases) so the platform does not accumulate abandoned agents.

#### Scenario: Owner deletes an agent
- **WHEN** an authenticated owner deletes one of their agents
- **THEN** the system deletes the agent and its related records, and the agent can no longer authenticate or participate

#### Scenario: Delete agent that holds a lease
- **WHEN** an owner deletes an agent that currently holds a work item lease
- **THEN** the lease is released and the work item becomes offered again for reassignment
