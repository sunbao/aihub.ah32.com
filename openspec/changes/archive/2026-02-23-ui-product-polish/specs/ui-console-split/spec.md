# ui-console-split (UI Product Polish)

## ADDED Requirements

### Requirement: Default UI does not show internal identifiers
The system SHALL NOT display internal identifiers (UUIDs, run_id, work_item_id, moderation queue item IDs) in default console/admin views; internal IDs MAY be shown only in an explicit debug view.

#### Scenario: Admin task assignment list hides IDs by default
- **WHEN** an admin views the task assignment list
- **THEN** items show status/time/summary and do not display work_item_id/run_id by default

#### Scenario: Debug view can reveal raw JSON with IDs
- **WHEN** an admin toggles an explicit "debug/raw data" view
- **THEN** the UI may show raw JSON that includes internal IDs for troubleshooting

### Requirement: UI copy is Chinese-first
The console and admin pages SHALL use Chinese-first copy and SHALL avoid mixing English UI strings.

#### Scenario: Admin pages are Chinese-first
- **WHEN** a user opens `/ui/admin.html` or `/ui/admin-assign.html`
- **THEN** primary headings, buttons, empty states, and error messages are Chinese-first

### Requirement: Console and admin pages share consistent layout and spacing
The console and admin pages SHALL use consistent header/navigation layout and spacing so the experience feels cohesive (mobile-first).

#### Scenario: Headers are consistent across pages
- **WHEN** a user navigates between console and admin pages
- **THEN** the header layout (title + navigation) remains visually consistent and mobile-friendly

