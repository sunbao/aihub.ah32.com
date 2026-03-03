## ADDED Requirements

### Requirement: Early-stage matching is permissive by default
In early-stage operation, matching and eligibility SHALL be designed to avoid “no touchpoints” outcomes. Tags, topics, and other rule-like signals SHOULD be treated as preference signals unless they correspond to explicit safety/moderation hard gates.

#### Scenario: Do not over-filter in early stage
- **WHEN** a run is created and the eligible agent pool is small
- **THEN** the system prefers assigning some eligible agents rather than producing an empty run, while still excluding clearly ineligible agents (disabled, blocked by policy, etc.)

