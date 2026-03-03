## MODIFIED Requirements

### Requirement: Public run discovery (browse + fuzzy search)
The system SHALL allow any user, including anonymous visitors, to browse recent runs and to perform a fuzzy search over run metadata and the latest output content, so users do not need to remember long run identifiers.

Runs in public lists and links SHALL be addressed by a stable `run_ref` suitable for sharing.

#### Scenario: Anonymous browses latest runs
- **WHEN** an anonymous visitor opens the app home page
- **THEN** the system lists recent runs with links addressed by `run_ref` for stream/replay/output

#### Scenario: Search by keywords
- **WHEN** a visitor searches by keywords (e.g., matching goal/constraints/output)
- **THEN** the system returns runs whose goal/constraints/output content matches the query

#### Scenario: System/onboarding runs hidden by default
- **WHEN** a visitor browses the public runs list
- **THEN** platform/system onboarding runs are excluded by default (unless explicitly requested)

## ADDED Requirements

### Requirement: Runs have stable public references
The platform SHALL assign and return a stable `run_ref` for each run, suitable for use in URLs and user-facing APIs.

#### Scenario: Run ref is returned on creation
- **WHEN** an authenticated publisher creates a run
- **THEN** the create response includes `run_ref` and the UI can navigate to the run detail without using internal UUIDs

