# task-orchestration

## Purpose
Represent and progress agent work through stages and work items, prioritizing autonomous execution in MVP while supporting retries and reassignment.
## Requirements
### Requirement: Stage-based progression
The system SHALL represent agent work using named stages (e.g., ideation, drafting) and SHALL allow stage changes to be reflected in the collaboration stream to support clear live viewing and replay.

#### Scenario: Stage change event
- **WHEN** a participating agent emits a `stage_changed` event for a run
- **THEN** the system records the stage change event in the collaboration stream for live/replay rendering

### Requirement: Work item model
The system SHALL represent agent work as discrete work items that can be offered, claimed, completed, and retried, with comprehensive context for task completion.

#### Scenario: Create a work item
- **WHEN** the system needs an agent contribution for a stage
- **THEN** the system creates a work item associated with the run and stage, including goal, constraints, and stage-specific context

#### Scenario: Work item contains context fields
- **WHEN** a work item is created
- **THEN** it includes: goal (from run), constraints (from run), stage_description, expected_output, available_skills, previous_artifacts references

### Requirement: Autonomous execution preference
The system SHALL prioritize agent autonomous execution and SHALL NOT require human approval between work items or stages in MVP.

#### Scenario: No human approval step
- **WHEN** a publisher creates a run and watches it progress
- **THEN** the system does not present any “approve/continue” controls between stages/work items

### Requirement: Retry and fallback
The system SHALL support retrying or reassigning a work item when it fails or times out.

#### Scenario: Work item times out
- **WHEN** a claimed work item exceeds its lease time without completion
- **THEN** the system reoffers or reassigns the work item

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

