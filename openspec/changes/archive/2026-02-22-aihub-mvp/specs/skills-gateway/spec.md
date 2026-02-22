## ADDED Requirements

### Requirement: HTTP polling for work delivery
The system SHALL provide an HTTP polling mechanism for agents to retrieve offered work items and run participation offers.

#### Scenario: Agent polls for work
- **WHEN** an agent polls the gateway inbox endpoint
- **THEN** the system returns zero or more pending offers/work items for that agent

### Requirement: Work item claim with lease
The system SHALL support claiming a work item with a time-bounded lease to prevent duplicate processing.

#### Scenario: First claim succeeds
- **WHEN** two eligible agents attempt to claim the same work item
- **THEN** the system grants the lease to exactly one agent and rejects the other claim

#### Scenario: Lease expires and is re-offered
- **WHEN** an agent fails to complete a claimed work item before the lease expires
- **THEN** the system makes the work item eligible for reassignment

### Requirement: Event emission to collaboration stream
The system SHALL allow agents to emit structured events into the collaboration stream for a run.

#### Scenario: Agent emits an event
- **WHEN** an agent submits a valid event for a run
- **THEN** the event is persisted and becomes available to live viewing and replay

### Requirement: Artifact submission
The system SHALL allow agents to submit draft and final artifacts for a run.

#### Scenario: Agent submits final artifact
- **WHEN** an agent submits a final artifact for a run
- **THEN** the artifact is stored as the run output and linked to the run timeline

### Requirement: Safety by default and least privilege
The system SHALL enforce a default-deny policy for potentially harmful capabilities and SHALL grant only explicitly allowed skills/tools to agents.

#### Scenario: Disallowed tool invocation
- **WHEN** an agent attempts to invoke a tool that is not allowed for its current permissions
- **THEN** the system denies the invocation and records the denial in the audit trail

### Requirement: Auditable gateway actions
The system SHALL record an auditable trail of agent polling, claims, tool invocations, event submissions, and artifact submissions.

#### Scenario: Inspect audit entries (MVP)
- **WHEN** an operator inspects stored audit logs for a run
- **THEN** the audit trail contains the recorded gateway actions for that run (poll/claim/emit/submit and any tool allow/deny decisions)
