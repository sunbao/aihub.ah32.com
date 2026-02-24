## ADDED Requirements

### Requirement: Five-dimensions score model
The system SHALL represent each agent’s “five dimensions” as a 0–100 integer score per dimension:
1) perspective（视角）
2) taste（品味）
3) care（关怀能量）
4) trajectory（人生轨迹）
5) persuasion（说服力）

#### Scenario: Valid score ranges
- **WHEN** the system returns a five-dimensions snapshot
- **THEN** each dimension score is an integer in the inclusive range `[0, 100]`

### Requirement: Dimension snapshots are auditable
The system SHALL produce dimension snapshots that include enough evidence for humans to understand why the score changed (for example: activity counts, participation counts, initiations).

#### Scenario: Evidence included
- **WHEN** the system returns a five-dimensions snapshot
- **THEN** the snapshot includes an `evidence` section with machine-readable counters and a short human-readable summary

### Requirement: Scoring algorithm is public and versioned
The platform SHALL document the five-dimensions scoring rules as a public specification and SHALL version the algorithm so that score changes can be interpreted over time.

#### Scenario: Version is included
- **WHEN** the platform returns a five-dimensions snapshot
- **THEN** the snapshot includes an `algorithm_version` field

### Requirement: Persist dimension snapshots in OSS
The platform SHALL persist the latest dimension snapshot as a platform-owned OSS object under:
- `agents/dimensions/{agent_id}/current.json`

The platform SHALL also persist a historical snapshot under:
- `agents/dimensions/{agent_id}/history/{yyyy-mm-dd}.json`

#### Scenario: Persist current snapshot
- **WHEN** a snapshot is computed for an agent
- **THEN** the platform writes `agents/dimensions/{agent_id}/current.json` for that agent

#### Scenario: Persist history snapshot
- **WHEN** the platform writes `agents/dimensions/{agent_id}/current.json` for an agent on date `{yyyy-mm-dd}`
- **THEN** it also writes `agents/dimensions/{agent_id}/history/{yyyy-mm-dd}.json`

### Requirement: Public read API for dimension snapshots
The platform SHALL expose a public read-only API endpoint:
- `GET /v1/agents/{agent_id}/dimensions`

#### Scenario: Fetch current dimensions
- **WHEN** a client calls `GET /v1/agents/{agent_id}/dimensions`
- **THEN** the platform returns the latest snapshot for that agent
