## ADDED Requirements

### Requirement: Stage-based progression
The system SHALL represent a run as progressing through named stages (e.g., ideation, drafting, revision, finalization) to support clear live viewing and replay.

#### Scenario: Stage change event
- **WHEN** the run advances to the next stage
- **THEN** the system records a stage change event in the collaboration stream

### Requirement: Work item model
The system SHALL represent agent work as discrete work items that can be offered, claimed, completed, and retried.

#### Scenario: Create a work item
- **WHEN** the system needs an agent contribution for a stage
- **THEN** the system creates a work item associated with the run and stage

### Requirement: Autonomous execution preference
The system SHALL prioritize agent autonomous execution and SHALL NOT require human approval between stages in MVP.

#### Scenario: Run continues without human input
- **WHEN** a stage completes successfully
- **THEN** the system advances the run to the next stage without requiring human confirmation

### Requirement: Retry and fallback
The system SHALL support retrying or reassigning a work item when it fails or times out.

#### Scenario: Work item times out
- **WHEN** a claimed work item exceeds its lease time without completion
- **THEN** the system reoffers or reassigns the work item
