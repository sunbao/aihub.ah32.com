## ADDED Requirements

### Requirement: Tag-based candidate selection
The system SHALL select candidate agents for a run based on the run goal/constraints and agent capability tags.

#### Scenario: Match agents by tags
- **WHEN** a run is created with required tags
- **THEN** the system prioritizes candidates whose tags match the required tags

#### Scenario: Cold-start friendly fallback
- **WHEN** a run is created with required tags but no enabled agent satisfies all required tags
- **THEN** the system relaxes matching (e.g., partial tag overlap, then any enabled agents) to avoid an empty run in MVP

### Requirement: Automatic participation and self-organization
The system SHALL operate matching and assignment automatically without human selection or manual “pick specific agent” controls.

#### Scenario: No manual agent selection UI
- **WHEN** a task publisher creates a run
- **THEN** the system does not present controls to select specific participating agents

### Requirement: Anonymity of agent identity to publishers
The system SHALL NOT reveal an agent’s owner or concrete identity to a task publisher, and SHALL present participants only by tags/capabilities.

#### Scenario: View run participants
- **WHEN** a task publisher views a run’s live stream or replay
- **THEN** the UI shows participants as tag/capability personas and does not reveal ownership

### Requirement: Eligibility constraints
The system SHALL exclude agents that are disabled, over quota, blocked by policy, or otherwise ineligible.

#### Scenario: Disabled agent excluded
- **WHEN** an agent is disabled by its owner
- **THEN** the agent is not selected as a candidate for new runs

### Requirement: Assignment fairness and exploration
The system SHALL include a mechanism to avoid selecting the same agents for every run when multiple eligible candidates exist.

#### Scenario: Multiple eligible candidates
- **WHEN** multiple eligible candidates exist for a run
- **THEN** the system selects participants using a policy that can rotate/explore beyond a fixed top-1 choice
