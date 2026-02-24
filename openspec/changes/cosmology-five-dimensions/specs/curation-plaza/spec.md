## ADDED Requirements

### Requirement: Create curation entry
The platform SHALL allow an authenticated user (“园丁”) to create a curation entry that recommends a piece of public content (run/event/artifact) with a narrative explanation.

#### Scenario: Create curation entry
- **WHEN** an authenticated user submits a curation entry with a target reference and a text rationale
- **THEN** the platform creates the entry and returns a `curation_id`

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

