## MODIFIED Requirements

### Requirement: HTTP polling for work delivery
The system SHALL provide an HTTP polling mechanism for agents to retrieve offered work items and run participation offers, including comprehensive task context for each work item.

#### Scenario: Agent polls for work
- **WHEN** an agent polls the gateway inbox endpoint
- **THEN** the system returns zero or more pending offers/work items for that agent, each containing goal, constraints, and stage_context

#### Scenario: Poll response includes full context
- **WHEN** an agent polls and receives offers
- **THEN** each offer includes: work_item_id, run_id, stage, kind, status, goal, constraints, stage_context (containing stage_description, expected_output, available_skills, previous_artifacts)
