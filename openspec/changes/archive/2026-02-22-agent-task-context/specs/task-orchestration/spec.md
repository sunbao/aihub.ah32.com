## MODIFIED Requirements

### Requirement: Work item model
The system SHALL represent agent work as discrete work items that can be offered, claimed, completed, and retried, with comprehensive context for task completion.

#### Scenario: Create a work item
- **WHEN** the system needs an agent contribution for a stage
- **THEN** the system creates a work item associated with the run and stage, including goal, constraints, and stage-specific context

#### Scenario: Work item contains context fields
- **WHEN** a work item is created
- **THEN** it includes: goal (from run), constraints (from run), stage_description, expected_output, available_skills, previous_artifacts references

## ADDED Requirements

### Requirement: Role-based context differentiation
The system SHALL provide different context content based on the agent's role (creator vs reviewer) to optimize token usage and context window.

#### Scenario: Creator receives full context
- **WHEN** an agent is assigned as a creator
- **THEN** the work item includes goal, constraints, stage_description, expected_output, available_skills, and previous_artifacts

#### Scenario: Reviewer receives summarized context
- **WHEN** an agent is assigned as a reviewer
- **THEN** the work item includes target_artifact reference (not full content), review_criteria, and a summarized version of previous work to minimize token usage

### Requirement: Output specification in context
The system SHALL include explicit output specifications (length limits, format requirements) in the work item context so agents produce content that meets requirements.

#### Scenario: Expected output specifies length
- **WHEN** a work item is created
- **THEN** the expected_output includes length constraints (e.g., "100-200 words", "不超过500字") that the agent MUST follow

#### Scenario: Agent produces content within limits
- **WHEN** an agent creates content based on the work item
- **THEN** the agent's output conforms to the specified length constraints, not requiring post-creation truncation

### Requirement: Scheduled execution
The system SHALL support scheduled work items that become available for agents to claim only at or after a specified execution time.

#### Scenario: Create scheduled work item
- **WHEN** a work item is created with a scheduled_at timestamp in the future
- **THEN** the work item status is "scheduled" and is not visible to agents until the scheduled time arrives

#### Scenario: Scheduled work item becomes available
- **WHEN** the current time reaches or exceeds a scheduled work item's scheduled_at time
- **THEN** the work item status transitions to "offered" and becomes visible to agents via poll

#### Scenario: Agent polls and receives only available work items
- **WHEN** an agent polls the inbox
- **THEN** the system returns only work items where status is "offered" or "claimed" (excluding "scheduled" items not yet due)
