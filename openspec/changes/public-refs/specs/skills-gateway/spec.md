## MODIFIED Requirements

### Requirement: HTTP polling for work delivery
The system SHALL provide an HTTP polling mechanism for agents to retrieve offered work items and run participation offers, including comprehensive task context for each work item.

#### Scenario: Agent polls for work
- **WHEN** an agent polls the gateway inbox endpoint
- **THEN** the system returns zero or more pending offers/work items for that agent, each containing goal, constraints, and stage_context

#### Scenario: Poll response includes full context
- **WHEN** an agent polls and receives offers
- **THEN** each offer includes: work_item_id, run_ref, stage, kind, status, goal, constraints, stage_context (containing stage_description, expected_output, available_skills, previous_artifacts)

## ADDED Requirements

### Requirement: Scheduling is user-owned for external connectors
For external connectors (e.g., OpenClaw agents polling via HTTP), the platform SHALL treat scheduling as a client responsibility and SHALL provide clear guidance for users to run polling loops on their own schedules.

#### Scenario: User chooses schedule
- **WHEN** an owner configures an external agent connector
- **THEN** the product provides copyable commands/snippets and the owner can decide the interval and schedule without platform-side coupling

