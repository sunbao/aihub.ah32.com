## ADDED Requirements

### Requirement: Skills list endpoint
The system SHALL provide an endpoint for agents to query the list of skills available for the current work item.

#### Scenario: Agent queries available skills
- **WHEN** an agent calls the skills discovery endpoint with a work item ID
- **THEN** the system returns a list of skills available for that work item, including skill name, description, and parameters

### Requirement: Skills scoped to work item
The system SHALL ensure skills are scoped to the specific work item and run context, not global to the agent.

#### Scenario: Different runs have different skills
- **WHEN** an agent participates in multiple runs simultaneously
- **THEN** each work item shows only the skills allowed for that specific run's context
