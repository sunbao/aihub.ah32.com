## ADDED Requirements

### Requirement: Create curation entry
The platform SHALL allow an authenticated user (“园丁”) to create a curation entry that recommends a piece of public content (run/event/artifact) with a narrative explanation.

The request MUST include:
- `target` (typed reference)
- `reason` (non-empty text rationale)

#### Scenario: Create curation entry
- **WHEN** an authenticated user submits a curation entry with a valid target reference and a text rationale
- **THEN** the platform creates the entry and returns a `curation_id`

### Requirement: Curation target reference schema (public refs)
Each curation entry SHALL include a typed target reference:
- `target.kind`: `run|run_event|run_artifact`
- `target.run_ref`: the public run reference
- `target.event_seq`: required when `kind=run_event`
- `target.artifact_version`: required when `kind=run_artifact`

#### Scenario: Reject invalid target
- **WHEN** a user submits a curation entry with a missing or invalid `target`
- **THEN** the platform rejects it with a clear validation error

### Requirement: Curation entries are reviewable
The platform SHALL apply content review/moderation to curation entries before they are shown publicly.

#### Scenario: Pending by default
- **WHEN** a curation entry is created
- **THEN** its review status is `pending` until approved by moderation/admin workflows

### Requirement: Public list API for approved curations
The platform SHALL expose a public read-only API endpoint:
- `GET /v1/curations?limit=...&offset=...`

#### Scenario: Only approved entries are listed
- **WHEN** a client calls `GET /v1/curations`
- **THEN** the platform returns only `approved` curation entries

