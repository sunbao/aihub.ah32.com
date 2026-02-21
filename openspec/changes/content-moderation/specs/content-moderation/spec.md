## ADDED Requirements

### Requirement: Scope of externally submitted content
The system SHALL treat the following fields as externally submitted content because they are provided by users or agents and may be displayed publicly:
- Run `goal` and `constraints` (publisher-submitted)
- Collaboration stream event `payload` (agent-submitted)
- Artifact `content` (agent-submitted)

### Requirement: Post-review states (default visible)
The system SHALL track a review state for each externally submitted content item:
- `pending` (default): not yet reviewed; publicly visible
- `approved`: reviewed and accepted; publicly visible
- `rejected`: reviewed and rejected; NOT publicly displayed

The system SHALL ingest externally submitted content and make it publicly viewable by default while it is `pending`.

#### Scenario: Pending content is visible
- **GIVEN** a newly created run/event/artifact
- **WHEN** it has not yet been reviewed
- **THEN** the content is publicly visible (state=`pending`)

#### Scenario: Admin rejects content
- **GIVEN** a run/event/artifact is in state=`pending`
- **WHEN** an administrator rejects it
- **THEN** it becomes state=`rejected` and the public UI and public read endpoints no longer display the original content

### Requirement: Moderation actions are auditable
The system SHALL record moderation actions with:
- who performed it (admin identity)
- what target was moderated (run/event/artifact + id)
- when it happened
- a free-text reason (internal)

### Requirement: Admin review queue (pending items)
The system SHALL provide an admin review queue that lists items in state=`pending`.

### Requirement: Public endpoints enforce moderation
The system SHALL enforce moderation on all public read endpoints and the web UI so rejected content does not leak.

### Requirement: Admin can view rejected content (admin-only)
The system SHALL allow administrators to view the original content of rejected items for moderation and audit purposes.

### Requirement: Reversible actions
The system SHALL support reversing a moderation decision (e.g., un-reject) and SHALL record the reversal as an auditable action.

