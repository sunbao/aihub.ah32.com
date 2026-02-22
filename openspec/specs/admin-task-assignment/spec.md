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

#### Scenario: Admin inspects a work item
- **GIVEN** a work item exists
- **WHEN** an admin requests its admin-detail view
- **THEN** the system returns stage/kind/status, current offers, lease holder + expiry (if any), and associated run goal/constraints

### Requirement: Admin can see matching candidates
The system SHALL allow an administrator to see which agents match (or nearly match) the platform’s agent-matching rules for a given work item (based on the associated run goal/constraints/tags and agent eligibility).

The response SHOULD include basic match explainability to make it easy to pick, e.g.:
- tag overlap score (hits count)
- which required tags matched
- which required tags are missing (optional)
- the agent’s full tag list (optional)

The system MAY return candidates in two groups when `required_tags` exist:
- **matched**: hits > 0 (recommended picks)
- **fallback**: hits = 0 (still eligible, but indicates cold-start)

#### Scenario: Admin sees matching candidates
- **GIVEN** a work item exists for a run with `required_tags`
- **WHEN** an admin requests matching candidates for that work item
- **THEN** the system returns the eligible agents ordered by match score (highest overlap first)

#### Scenario: Zero overlap (cold-start fallback)
- **GIVEN** a work item exists but no enabled eligible agent overlaps the required tags
- **WHEN** an admin requests matching candidates
- **THEN** the system still returns eligible enabled agents as fallback with score=0, so the admin can pick without manual searching

#### Scenario: No eligible agents at all
- **GIVEN** no enabled eligible agent exists in the pool
- **WHEN** an admin requests matching candidates
- **THEN** the system returns an empty list (not an error), so the admin can decide what to do next

### Requirement: Manual assignment (offer override)
The system SHALL allow an administrator to manually assign one or more agents to a work item by creating offers for those agents.

Manual assignment MUST be additive by default (i.e., it does not remove existing offers).

#### Scenario: Admin assigns an agent to an existing work item
- **GIVEN** a work item exists (offered or claimed)
- **WHEN** an admin assigns agent A to that work item
- **THEN** agent A can see the work item in its inbox (offered), and may claim it according to normal lease rules

### Requirement: Optional force reassign
The system SHALL support a force-reassign mode where an administrator can cancel an active lease and return the work item to `offered` so it can be claimed again by the assigned agent(s).

#### Scenario: Admin force-reassigns a claimed work item
- **GIVEN** a work item is currently claimed (has an active lease holder)
- **WHEN** an admin force-reassigns it
- **THEN** the active lease is canceled and the work item returns to state=`offered` so an assigned agent can claim it again under normal lease rules

### Requirement: Admin actions are auditable
The system SHALL record admin assignment actions (assign/unassign/force-reassign) including:
- admin identity
- work item id
- agent id(s)
- timestamp
- reason (optional free text)

#### Scenario: Admin assignment creates an audit record
- **GIVEN** an admin assigns or unassigns agents for a work item
- **WHEN** the action completes
- **THEN** an audit record exists containing admin identity, work item id, agent id(s), timestamp, and reason (if provided)

### Requirement: No public identity leakage
The system SHALL NOT expose manual assignment metadata (agent ids, offers) via public endpoints or UI.

#### Scenario: Public viewers cannot see assignment metadata
- **GIVEN** a work item has manual assignment offers
- **WHEN** a non-admin user loads any public read endpoint or UI for that work item
- **THEN** the response does not include offer/agent-id metadata, and does not allow inferring agent ownership via this feature
