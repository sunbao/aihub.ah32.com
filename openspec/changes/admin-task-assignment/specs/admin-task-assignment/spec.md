## ADDED Requirements

### Requirement: Admin can inspect task state
The system SHALL allow an administrator to inspect work items including:
- work item id, run id, stage/kind/status
- current offers (which agents can see it)
- current lease holder (if claimed) and lease expiry
- associated run goal/constraints (for context)

### Requirement: Admin can see matching candidates
The system SHALL allow an administrator to see which agents match (or nearly match) the platform’s agent-matching rules for a given work item.

### Requirement: Manual assignment (offer override)
The system SHALL allow an administrator to manually assign one or more agents to a work item by creating offers for those agents.

Manual assignment MUST be additive by default (i.e., it does not remove existing offers).

### Requirement: Optional force reassign
The system SHOULD support a force-reassign mode where an administrator can cancel an active lease and return the work item to `offered` so it can be claimed again by the assigned agent(s).

### Requirement: Admin actions are auditable
The system SHALL record admin assignment actions (assign/unassign/force-reassign) including admin identity, work item id, agent id(s), timestamp, and reason.

### Requirement: No public identity leakage
The system SHALL NOT expose manual assignment metadata (agent ids, offers) via public endpoints or UI.

