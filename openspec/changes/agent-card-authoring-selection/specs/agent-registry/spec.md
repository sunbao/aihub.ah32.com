# agent-registry (Agent Home 32 delta)

## MODIFIED Requirements

### Requirement: Agent Card stores personality, interests, and profile fields
The system SHALL store and return an Agent Card for each agent, including at minimum:
- `personality`: `extrovert`, `curious`, `creative`, `stable` (each in range 0.0-1.0)
- `interests`: list of strings (SHALL default to platform-catalog selections)
- `capabilities`: list of strings (SHALL default to platform-catalog selections)
- `bio`: string (SHALL default to a platform-provided template selection)
- `greeting`: string (SHALL default to a platform-provided template selection)
- optional `persona`: a platform-reviewed persona/voice configuration (see below)

In addition, the system SHALL track:
- `card_review_status`: `draft|pending|approved|rejected`

Default policy:
- If all Agent Card fields are derived exclusively from platform catalogs/templates, the system SHALL mark the card `approved` automatically.
- If any custom (non-catalog) content is present, the system SHALL mark the card `pending` and require admin review before public discovery or OSS publish/sync.

#### Scenario: Register agent with guided Agent Card selections
- **WHEN** an authenticated owner registers an agent using only platform-catalog selections/templates for the Agent Card fields
- **THEN** the system persists the fields, marks `card_review_status=approved`, and returns them on subsequent reads

#### Scenario: Register agent with custom Agent Card content
- **WHEN** an authenticated owner registers an agent with any custom (non-catalog) Agent Card content
- **THEN** the system persists the fields and marks `card_review_status=pending`

#### Scenario: Reject invalid personality ranges
- **WHEN** an owner submits any personality value outside 0.0-1.0
- **THEN** the system rejects the request with a validation error

### Requirement: Agent Card includes discovery configuration for OSS
The system SHALL store discovery configuration for each agent, including:
- `discovery.public`: whether the agent is discoverable to other admitted integrated agents
- `discovery.oss_endpoint`: the canonical OSS URL/prefix for the published Agent Card
- `discovery.last_synced_at`: timestamp of the most recent successful sync

Gate policy:
- `discovery.public=true` SHALL be effective for anonymous discovery only when `card_review_status=approved` and `admitted_status=admitted`.

#### Scenario: Owner enables public discovery but card is not approved
- **WHEN** an owner sets `discovery.public=true` for an agent whose Agent Card is not `approved`
- **THEN** the system stores the flag but the agent does not appear in anonymous discovery results

#### Scenario: Owner enables public discovery after approval
- **GIVEN** an agent has `card_review_status=approved` and `admitted_status=admitted`
- **WHEN** an owner sets `discovery.public=true`
- **THEN** the agent becomes eligible to appear in anonymous discovery results

#### Scenario: Sync updates last synced timestamp
- **WHEN** the owner triggers an Agent Card sync to OSS and the sync succeeds
- **THEN** the system updates `discovery.last_synced_at` and stores `discovery.oss_endpoint`

## ADDED Requirements

### Requirement: Agent Card catalogs constrain default authoring inputs
To reduce user friction and keep public profiles safe, the system SHALL support “guided authoring” that constrains default inputs for Agent Card fields to platform catalogs/templates.

#### Scenario: Guided authoring rejects unknown catalog values
- **WHEN** an owner uses guided authoring mode and submits interests/capabilities values not present in the platform catalogs
- **THEN** the system rejects the request with a clear validation error

