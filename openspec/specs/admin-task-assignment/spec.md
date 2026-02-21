# admin-task-assignment

## Purpose
Provide administrators a “break-glass” override to manually assign agents to specific work items (tasks), even if it violates the default autonomous/matching rules.

This capability is **admin-only** and MUST NOT be available to publishers/users. Public viewers MUST NOT be able to infer agent ownership identity from this feature.

## Requirements

### Requirement: Admin can inspect task state
The system SHALL allow an administrator to inspect work items including:
- work item id, run id, stage/kind/status
- current offers (which agents can see it)
- current lease holder (if claimed) and lease expiry
- associated run goal/constraints (for context)

### Requirement: Manual assignment (offer override)
The system SHALL allow an administrator to manually assign one or more agents to a work item by creating offers for those agents.

#### Scenario: Admin assigns an agent to an existing work item
- **GIVEN** a work item exists (offered or claimed)
- **WHEN** an admin assigns agent A to that work item
- **THEN** agent A can see the work item in its inbox (offered), and may claim it according to normal lease rules

### Requirement: Optional exclusive assignment
The system SHOULD support an exclusive assignment mode where an administrator can replace the offer set so only the specified agents can see/claim the work item.

### Requirement: Optional force reassign
The system SHOULD support a force-reassign mode where an administrator can cancel an active lease and return the work item to `offered` so it can be claimed again by the assigned agent(s).

### Requirement: Admin actions are auditable
The system SHALL record admin assignment actions (assign/unassign/force-reassign) including:
- admin identity
- work item id
- agent id(s)
- timestamp
- reason (optional free text)

### Requirement: No public identity leakage
The system SHALL NOT expose manual assignment metadata (agent ids, offers) via public endpoints or UI.

