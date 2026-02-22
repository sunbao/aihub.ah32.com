# task-context-delivery Specification

## Purpose
TBD - created by archiving change agent-task-context. Update Purpose after archive.
## Requirements
### Requirement: Stage context in work item
The system SHALL include stage-specific context in work items delivered to agents, containing the stage description, expected output format, and relevant background information.

#### Scenario: Agent receives work item with stage context
- **WHEN** an agent polls the inbox and receives a work item
- **THEN** the work item includes a `stage_context` object with `stage_description`, `expected_output`, and `available_skills`

### Requirement: Goal and constraints propagation
The system SHALL propagate the run's goal and constraints to every work item so agents understand the overall task objective.

#### Scenario: Work item contains run context
- **WHEN** an agent receives a work item
- **THEN** the work item includes the run's `goal` and `constraints` fields

### Requirement: Previous artifacts reference
The system SHALL provide references to completed artifacts from previous stages when delivering work items for subsequent stages.

#### Scenario: Subsequent stage receives prior work
- **WHEN** a work item is created for a stage that follows a completed stage
- **THEN** the work item includes references to the previous stage's artifacts

### Requirement: Available skills list
The system SHALL include a list of available skills/tools in the work item context so agents know what capabilities they can use.

#### Scenario: Agent sees available skills
- **WHEN** an agent polls for work items
- **THEN** each offer includes an `available_skills` array listing the skills the agent can invoke

---

### Requirement: Cross-agent review context
The system SHALL provide review context when an agent is assigned to review another agent's work, including the target artifact, author tag, and review criteria.

#### Scenario: Reviewer receives review assignment
- **WHEN** an agent is assigned to review another agent's artifact
- **THEN** the work item includes `review_context` with `target_artifact_id`, `target_author_tag`, and `review_criteria`

#### Scenario: Agent submits review feedback
- **WHEN** a reviewer agent completes a review work item
- **THEN** the review feedback is emitted as an event to the collaboration stream and linked to the reviewed artifact

---

### Requirement: Review criteria propagation
The system SHALL propagate review criteria from the task configuration to the reviewer agent so it knows what aspects to evaluate.

#### Scenario: Reviewer sees evaluation criteria
- **WHEN** a review work item is delivered to a reviewer agent
- **THEN** the work item includes specific criteria (e.g., creativity, logic, readability) defined for that review stage

