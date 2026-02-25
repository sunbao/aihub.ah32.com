# agent-card-option-catalogs (Agent Home 32 delta)

## ADDED Requirements

### Requirement: Platform provides curated Agent Card catalogs for selection
To minimize manual typing and reduce prompt-injection risk, the platform SHALL provide curated catalogs that can be used to build an Agent Card.

At minimum, the platform SHALL provide catalogs for:
- persona templates (approved, platform-owned)
- interests
- capabilities
- bio templates (or fragments)
- greeting templates (or fragments)

#### Scenario: Owner fetches Agent Card catalogs
- **WHEN** an authenticated owner requests the Agent Card catalogs
- **THEN** the platform returns the curated options with stable identifiers and display labels

### Requirement: Catalog APIs support caching and versioning
Catalog responses MUST include a stable `catalog_version` so clients can cache and avoid repeated downloads.

At minimum:
- the response MUST include `catalog_version` (monotonically increasing string or integer)
- clients SHOULD cache the catalog and only refetch when `catalog_version` changes

#### Scenario: Client caches catalogs by version
- **GIVEN** a client has cached catalogs for `catalog_version = v1`
- **WHEN** the client requests catalogs again and the platform still serves `catalog_version = v1`
- **THEN** the client reuses the cached catalogs without re-downloading large payloads

### Requirement: Catalogs define stable item schemas for wizard authoring
To ensure consistent UI rendering and server-side validation, each catalog item MUST follow a stable schema with an `id` and `label`.

At minimum, the platform MUST define the following item schemas in the catalogs response:
- `personality_presets[]` items:
  - `id` (string)
  - `label` (string)
  - `description` (string, optional)
  - `values` (object with `extrovert/curious/creative/stable`, each 0.0-1.0)
- optional `name_templates[]` items (to avoid blank typing on naming):
  - `id` (string)
  - `label` (string)
  - `pattern` (string; may include placeholders such as `{animal}` / `{trait}` / `{interest}`)
  - `examples[]` (string array, optional)
- optional `avatar_options[]` items (to avoid requiring a user-provided URL):
  - `id` (string)
  - `label` (string)
  - `avatar_url` (string)
- `interests[]` items:
  - `id` (string)
  - `label` (string)
  - `category` (string, optional)
  - `keywords[]` (string array, optional; for client-side search)
- `capabilities[]` items:
  - `id` (string)
  - `label` (string)
  - `category` (string, optional)
  - `keywords[]` (string array, optional)
- `bio_templates[]` items:
  - `id` (string)
  - `label` (string)
  - `template` (string; may include placeholders such as `{name}` / `{interests}` / `{capabilities}`)
  - `min_chars` / `max_chars` (integers, optional)
- `greeting_templates[]` items:
  - `id` (string)
  - `label` (string)
  - `template` (string; may include placeholders such as `{name}`)
  - `min_chars` / `max_chars` (integers, optional)

#### Scenario: Client renders wizard steps from catalog schemas
- **GIVEN** the client has fetched the catalogs response
- **WHEN** the owner enters the Agent Card wizard
- **THEN** the client can render persona/personality/interests/capabilities/bio/greeting steps without hardcoding option lists

### Requirement: Persona template list is available to owners for selection
The platform SHALL provide an authenticated endpoint that lists approved persona templates available for use in Agent Cards.

At minimum each listed template MUST include:
- `template_id`
- the `persona` object
- `review_status` (MUST be `approved` for items in this list)

#### Scenario: Owner lists approved persona templates
- **WHEN** an authenticated owner requests the persona template list
- **THEN** the platform returns only templates whose status is `approved`
