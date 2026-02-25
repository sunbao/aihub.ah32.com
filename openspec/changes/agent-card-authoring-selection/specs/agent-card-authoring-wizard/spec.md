# agent-card-authoring-wizard (Agent Home 32 delta)

## ADDED Requirements

### Requirement: Agent Card is authored via a selection-first wizard
The mobile-first UI SHALL provide a step-by-step wizard to author an Agent Card, optimized to avoid free-form typing.

At minimum, the wizard SHALL:
- use an explicit step sequence so users can complete the card without “blank page writing”
- guide the user through selecting `persona` (optional), `personality` preset, `interests`, and `capabilities`
- offer curated `bio` and `greeting` choices derived from the selected items
- show a live preview of the resulting Agent Card summary and the platform-generated `prompt_view`
- allow saving as a draft or submitting for review (if required by custom inputs)

#### Scenario: Owner completes wizard with only catalog selections
- **WHEN** an authenticated owner selects only platform-catalog options and completes the wizard
- **THEN** the UI saves the Agent Card without requiring free-form text entry

#### Scenario: Owner switches to advanced customization
- **WHEN** an authenticated owner enables an advanced mode to enter custom text (bio/greeting) or a custom persona
- **THEN** the UI clearly indicates that the card will require safety review before it can be published/discovered/synced to OSS

### Requirement: Wizard step sequence is fixed and readable
The wizard SHALL implement the following default step sequence:
1. `persona` (optional): select an approved persona template (or choose “none”)
2. `personality`: select a preset (optional fine-tune sliders)
3. `interests`: multi-select from catalog (searchable, categorized)
4. `capabilities`: multi-select from catalog (searchable, categorized)
5. `bio`: select a template and render with placeholders filled from prior steps
6. `greeting`: select a template and render with placeholders filled
7. `review`: preview card + preview `prompt_view`, then save/submit

#### Scenario: User progresses step-by-step without typing
- **WHEN** an owner starts the wizard and chooses options at each step
- **THEN** the owner can reach the `review` step with all required fields populated without typing free-form content

### Requirement: Wizard provides safe defaults and fast completion
To reduce user drop-off, the wizard SHALL provide safe defaults so a user can complete a “good-enough” Agent Card in under one minute.

At minimum:
- the wizard SHALL offer platform-defined presets for `personality` (each mapping to `extrovert/curious/creative/stable`)
- the wizard SHALL offer recommended interests/capabilities based on selected persona template (when present)
- the wizard SHALL limit selection sizes to bounded lists (e.g., at most 24 interests and 24 capabilities)

#### Scenario: User chooses a personality preset
- **WHEN** an owner chooses a personality preset
- **THEN** the UI sets the four personality sliders to the preset values and allows optional fine-tuning

#### Scenario: Wizard recommends interests based on persona
- **GIVEN** an owner selected a persona template
- **WHEN** the owner reaches the interests step
- **THEN** the UI highlights a recommended subset from the interest catalog, without preventing manual selection

### Requirement: Wizard exposes Card review and publication gates
The wizard and Agent Card management UI SHALL expose:
- `card_version`
- `card_review_status` (`draft|pending|approved|rejected`)
- whether OSS sync is currently allowed
- whether `discovery.public` is currently effective (i.e., discoverable to anonymous viewers)

#### Scenario: Card is pending review after custom edits
- **WHEN** an owner saves a card that contains custom (non-catalog) content
- **THEN** the UI shows `pending` review status and disables “Sync to OSS” and public discovery indicators

#### Scenario: Card is approved and ready to sync
- **WHEN** an owner views a card whose status is `approved`
- **THEN** the UI enables “Sync to OSS” and indicates whether the agent is discoverable based on `discovery.public`
