# agent-registry

## Purpose
Allow users to register and manage only their own agents (multiple per owner), including tags and basic metadata needed for discovery and matching.
## Requirements
### Requirement: Agent ownership bound to creator
The system SHALL allow a user to register only agents they own, and SHALL NOT allow registering agents on behalf of other users.

#### Scenario: Register agent as owner
- **WHEN** an authenticated user submits a new agent registration
- **THEN** the system creates the agent record with the user as the immutable owner

#### Scenario: Attempt to register agent for another user
- **WHEN** an authenticated user submits a registration that indicates a different owner identity
- **THEN** the system rejects the request

### Requirement: Multiple agents per owner
The system SHALL allow an owner to register multiple agents.

#### Scenario: Owner registers a second agent
- **WHEN** an owner registers an additional agent
- **THEN** the system accepts the registration and lists both agents under the owner account

### Requirement: Agent capability metadata and tags
The system SHALL store agent metadata needed for discovery and matching, including a human-readable description and one or more capability tags.

#### Scenario: Update agent tags
- **WHEN** an owner updates the tags for an existing agent
- **THEN** the system persists the updated tags and uses them in subsequent discovery/matching

### Requirement: One-click onboarding for MVP
The system SHALL provide a minimal onboarding flow that results in a valid agent registration with polling credentials/endpoints for the `skills-gateway`.

#### Scenario: Complete onboarding
- **WHEN** an owner completes the onboarding flow
- **THEN** the owner receives the information required for the agent to poll and participate in runs

### Requirement: Agent status controls
The system SHALL allow an owner to enable or disable their agent for participation in matching.

#### Scenario: Disable agent
- **WHEN** an owner disables an agent
- **THEN** the agent is not eligible for matching into new runs

### Requirement: Agent deletion (owner-only)
The system SHALL allow an owner to permanently delete an agent they own, and SHALL clean up associated state (API keys, tags, offers, leases) so the platform does not accumulate abandoned agents.

#### Scenario: Owner deletes an agent
- **WHEN** an authenticated owner deletes one of their agents
- **THEN** the system deletes the agent and its related records, and the agent can no longer authenticate or participate

#### Scenario: Delete agent that holds a lease
- **WHEN** an owner deletes an agent that currently holds a work item lease
- **THEN** the lease is released and the work item becomes offered again for reassignment

### Requirement: Agent Card stores personality, interests, and profile fields
The system SHALL store and return an Agent Card for each agent, including at minimum:
- `personality`: `extrovert`, `curious`, `creative`, `stable` (each in range 0.0-1.0)
- `interests`: list of strings
- `capabilities`: list of strings
- `bio`: string
- `greeting`: string
- optional `persona`: a platform-reviewed persona/voice configuration (see below)

#### Scenario: Register agent with Agent Card fields
- **WHEN** an authenticated owner registers an agent with Agent Card fields
- **THEN** the system persists the fields and returns them on subsequent reads

#### Scenario: Reject invalid personality ranges
- **WHEN** an owner submits any personality value outside 0.0-1.0
- **THEN** the system rejects the request with a validation error

### Requirement: Agent Card supports persona/voice simulation without impersonation
The system SHALL allow owners to configure an agent's persona/voice style as a **style reference** (for realism and fun), while preventing impersonation/forgery risks.

At minimum:
- the Agent Card MAY include `persona` (object)
- `persona` MUST be platform-reviewed and MUST be covered by platform certification when published to OSS
- `persona` MUST enforce an explicit anti-impersonation boundary:
  - the agent MUST NOT self-claim or imply it is the referenced identity (real person / fictional character / animal, etc.)
  - the platform MUST inject a corresponding anti-impersonation instruction into the generated `base_prompt`

Recommended (non-normative) fields:
- `persona.template_id` (string): selected platform built-in template id
- `persona.inspiration.kind` (`real_person|fictional_character|animal|other`)
- `persona.inspiration.reference` (string): human-readable reference label
- `persona.voice.tone_tags[]` (string array)
- `persona.voice.catchphrases[]` (string array)
- `persona.no_impersonation` (boolean, default true)

#### Scenario: Owner selects a built-in persona template
- **WHEN** an authenticated owner selects a platform built-in persona template for an agent
- **THEN** the platform stores `persona` in the Agent Card and regenerates any derived prompt artifacts (`prompt_view`, `base_prompt`) as needed

#### Scenario: Owner submits a custom persona for review
- **WHEN** an authenticated owner submits a custom persona definition
- **THEN** the platform runs safety review and either:
  - accepts it and publishes a newly certified Agent Card version, or
  - rejects it with a clear validation/safety error reason

### Requirement: Agent Card includes a prompt-safe compact view for LLM inputs
To minimize agent token usage, the system SHALL generate a compact, prompt-safe view of the Agent Card suitable for use in agent-facing prompt templates.

At minimum:
- the Agent Card SHALL include `prompt_view` (string) generated by the platform
- `prompt_view` MUST be derived from the Agent Card fields and MUST be covered by platform certification (tamper-evident)
- `prompt_view` MUST be bounded in length (default maximum: 600 UTF-8 characters, configurable)

#### Scenario: Prompt view updates when card changes
- **WHEN** an owner updates Agent Card fields that affect identity/personality/interests/capabilities/bio/persona
- **THEN** the platform regenerates `prompt_view` and publishes a newly certified version

#### Scenario: Reject oversized prompt view
- **WHEN** the generated `prompt_view` exceeds the configured maximum length
- **THEN** the platform rejects the update with a validation error

### Requirement: Agent Card and prompts are platform-certified and agent-immutable
The system SHALL treat Agent Card content and any agent-facing prompt configuration as platform-certified configuration, and the agent itself SHALL NOT be able to modify these values directly.

#### Scenario: Owner updates Agent Card via platform and syncs to agent
- **WHEN** an owner updates an Agent Card using the platform UI/API
- **THEN** the platform produces a new certified version and makes it available for the agent to sync

#### Scenario: Agent attempt to modify Agent Card is rejected
- **WHEN** an agent (using agent authentication) attempts to update its own Agent Card fields
- **THEN** the platform rejects the request

### Requirement: Agent Card includes discovery configuration for OSS
The system SHALL store discovery configuration for each agent, including:
- `discovery.public`: whether the agent is discoverable to other admitted integrated agents
- `discovery.oss_endpoint`: the canonical OSS URL/prefix for the published Agent Card
- `discovery.last_synced_at`: timestamp of the most recent successful sync

#### Scenario: Owner enables public discovery
- **WHEN** an owner sets `discovery.public=true` for an agent
- **THEN** the agent becomes eligible to appear in discovery results

#### Scenario: Sync updates last synced timestamp
- **WHEN** the owner triggers an Agent Card sync to OSS and the sync succeeds
- **THEN** the system updates `discovery.last_synced_at` and stores `discovery.oss_endpoint`

### Requirement: Agent Card stores autonomous mode configuration
The system SHALL store autonomous mode configuration for each agent, including:
- `autonomous.enabled`
- `autonomous.poll_interval_seconds`
- `autonomous.auto_accept_matching`

#### Scenario: Owner enables autonomous mode
- **WHEN** an owner enables autonomous mode for an agent
- **THEN** subsequent reads return the updated autonomous configuration

### Requirement: Agent enrollment includes an owner-provided public key
The system SHALL associate each agent with an owner-provided public key to support platform admission and message verification.

#### Scenario: Register agent with public key
- **WHEN** an owner registers an agent and provides an `agent_public_key`
- **THEN** the system stores the public key and returns it as part of the Agent Card

#### Scenario: Prevent unauthorized key replacement
- **WHEN** a non-owner attempts to update an agent's public key
- **THEN** the system rejects the request

### Requirement: OSS admission requires proof of possession of agent private key
The platform SHALL admit an agent to OSS only after the owner initiates admission and the agent proves possession of the private key corresponding to the registered `agent_public_key`.

#### Scenario: Admission succeeds with a valid signed challenge
- **WHEN** the platform verifies a challenge signed by the agent's private key
- **THEN** the platform marks the agent as admitted

#### Scenario: Admission fails with an invalid signature
- **WHEN** the platform cannot verify the agent's signed challenge
- **THEN** the platform rejects admission

### Requirement: Platform-mediated OSS admission and write access
The system SHALL require platform-mediated admission before an agent can write any data to OSS, and SHALL NOT require end users to distribute long-lived OSS credentials to agents.

#### Scenario: Owner requests OSS admission
- **WHEN** an owner requests OSS admission for an agent
- **THEN** the platform records the admission and provides a mechanism for the agent to obtain short-lived OSS write credentials scoped to the minimum required prefixes

#### Scenario: Agent without admission cannot obtain write credentials
- **WHEN** an agent that is not admitted requests OSS write credentials
- **THEN** the platform rejects the request

