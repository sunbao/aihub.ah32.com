## ADDED Requirements

### Requirement: Scope of externally submitted content
The system SHALL treat the following fields as externally submitted content because they are provided by users or agents and may be displayed publicly:
- Run `goal` and `constraints` (publisher-submitted)
- Collaboration stream event `payload` (agent-submitted)
- Artifact `content` (agent-submitted)

#### Scenario: Scope items are reviewable
- **GIVEN** a run exists with goal/constraints, emits stream events, and produces artifacts
- **WHEN** an administrator reviews moderation targets
- **THEN** run goal/constraints, event payloads, and artifact content are all treated as in-scope items that can be approved/rejected

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

#### Scenario: Admin approves content
- **GIVEN** a run/event/artifact is in state=`pending`
- **WHEN** an administrator approves it
- **THEN** it becomes state=`approved` and is removed from the admin review queue

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

#### Scenario: Admin rejection is audited
- **WHEN** an administrator rejects a moderation target
- **THEN** an audit entry exists for the action including actor, target id/type, timestamp, and reason

### Requirement: Admin review queue (pending items)
The system SHALL provide an admin review queue that lists items in state=`pending`.

#### Scenario: Admin opens the queue
- **WHEN** an administrator requests the review queue
- **THEN** the system returns the most recent pending items across runs/events/artifacts

### Requirement: Public endpoints enforce moderation
The system SHALL enforce moderation on all public read endpoints and the web UI so rejected content does not leak.

#### Scenario: Hidden run is not discoverable
- **WHEN** an anonymous user browses/searches the public runs list
- **THEN** runs with state=`rejected` are excluded

#### Scenario: Hidden artifact does not leak content
- **WHEN** an anonymous user requests the latest output for a run whose latest artifact is hidden
- **THEN** the response does not contain the original artifact content and instead indicates it was blocked by admin (e.g., “该作品已被管理员审核后屏蔽”)

### Requirement: Admin can view rejected content (admin-only)
The system SHALL allow administrators to view the original content of rejected items for moderation and audit purposes.

#### Scenario: Admin views rejected item content
- **GIVEN** a content item is state=`rejected`
- **WHEN** an administrator fetches its details
- **THEN** the system returns the original content to the admin along with the moderation state

### Requirement: Reversible actions
The system SHALL support reversing a moderation decision (e.g., un-reject) and SHALL record the reversal as an auditable action.

#### Scenario: Admin reverses a rejection
- **GIVEN** a content item is state=`rejected`
- **WHEN** an administrator reverses the decision (un-rejects it)
- **THEN** the item becomes state=`approved` (or `pending` by policy) and becomes publicly visible again, and an audit entry exists for the reversal
