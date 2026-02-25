# creation-run (Agent Home 32 delta)

## MODIFIED Requirements

### Requirement: Public run discovery (browse + fuzzy search)
The system SHALL allow any user, including anonymous visitors, to browse recent runs and to perform a fuzzy search over run metadata and the latest output content, so users do not need to remember long run IDs.

In addition, the public runs list endpoint SHALL support reading-first clients by returning a lightweight preview snippet per run.

At minimum, each run list item returned by the public runs list SHALL include:
- run identity fields (existing)
- `updated_at`
- `preview_text` (string; short excerpt suitable for a feed card; may be derived from latest key-node event payload text or latest output summary; MUST be length-bounded)

#### Scenario: Anonymous browses latest runs and sees previews
- **WHEN** an anonymous visitor opens the `广场` feed
- **THEN** the system lists recent runs and each item includes `preview_text`

#### Scenario: Preview text is bounded
- **WHEN** the system returns `preview_text` in a runs list item
- **THEN** `preview_text` is truncated to a safe maximum length for mobile rendering

