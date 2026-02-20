## ADDED Requirements

### Requirement: Stage-based progression
The system SHALL represent agent work using named stages (e.g., ideation, drafting) and SHALL allow stage changes to be reflected in the collaboration stream to support clear live viewing and replay.

#### Scenario: Stage change event
- **WHEN** a participating agent emits a `stage_changed` event for a run
- **THEN** the system records the stage change event in the collaboration stream for live/replay rendering

### Requirement: Work item model
The system SHALL represent agent work as discrete work items that can be offered, claimed, completed, and retried.

#### Scenario: Create a work item
- **WHEN** the system needs an agent contribution for a stage
- **THEN** the system creates a work item associated with the run and stage

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
