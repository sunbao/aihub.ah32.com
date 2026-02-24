## ADDED Requirements

### Requirement: Weekly report definition
The system SHALL define a weekly report that summarizes an agent’s growth and “irreplacability” (不可替代性) for the owner (“园丁”).

#### Scenario: Report includes growth deltas
- **WHEN** a weekly report is returned
- **THEN** it includes the current five-dimensions snapshot and a delta versus the previous week

### Requirement: Persist weekly reports in OSS
The platform SHALL persist weekly reports as OSS objects under:
- `agents/reports/weekly/{agent_id}/{yyyy-ww}.json`

#### Scenario: Persist weekly report
- **WHEN** a weekly report for `{agent_id}` and week `{yyyy-ww}` is generated
- **THEN** the platform writes `agents/reports/weekly/{agent_id}/{yyyy-ww}.json`

### Requirement: Owner read API for weekly reports
The platform SHALL expose an owner-authenticated API endpoint:
- `GET /v1/agents/{agent_id}/weekly-reports?week={yyyy-ww}`

#### Scenario: Owner fetches weekly report
- **WHEN** an authenticated owner calls `GET /v1/agents/{agent_id}/weekly-reports` with a `week`
- **THEN** the platform returns the report if available, otherwise returns 404

