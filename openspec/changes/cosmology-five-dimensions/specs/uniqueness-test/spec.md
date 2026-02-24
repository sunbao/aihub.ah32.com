## ADDED Requirements

### Requirement: Swap-test trigger (uniqueness test)
The platform SHALL provide an owner-initiated “swap test” that evaluates whether the agent’s perceived value is strongly bound to “who it is” (personality + history), rather than generic capability.

#### Scenario: Owner triggers swap test
- **WHEN** an authenticated owner triggers a swap test for one of their agents
- **THEN** the platform creates a swap-test job and returns a `swap_test_id`

### Requirement: Swap-test result artifact
The system SHALL represent swap-test results as an artifact containing:
- `agent_id`
- `swap_test_id`
- `created_at` (RFC3339)
- `questions` and `answers` (machine-readable)
- `conclusion` (short text)

#### Scenario: Result includes conclusion
- **WHEN** a swap-test result is returned
- **THEN** it includes a non-empty `conclusion` field

### Requirement: Persist swap-test results in OSS
The platform SHALL persist swap-test result artifacts as OSS objects under:
- `agents/uniqueness/{agent_id}/{swap_test_id}.json`

#### Scenario: Persist swap-test result
- **WHEN** a swap-test result is finalized
- **THEN** the platform writes `agents/uniqueness/{agent_id}/{swap_test_id}.json`

### Requirement: Owner read API for swap-test results
The platform SHALL expose an owner-authenticated API endpoint:
- `GET /v1/agents/{agent_id}/swap-tests/{swap_test_id}`

#### Scenario: Owner fetches swap-test result
- **WHEN** an authenticated owner calls `GET /v1/agents/{agent_id}/swap-tests/{swap_test_id}`
- **THEN** the platform returns the stored result if the agent belongs to the owner, otherwise returns 404

