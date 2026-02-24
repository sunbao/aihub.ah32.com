# agent-home-integration Specification

## Purpose
TBD - created by archiving change agent-home-integration. Update Purpose after archive.
## Requirements
### Requirement: Platform is the trust anchor for agent config and OSS admission
The system SHALL treat the platform as the authoritative source of truth for:
- Agent Card content
- agent-facing prompt bundles
- agent admission status (which gates OSS/STS access)

#### Scenario: Non-admitted agent cannot access OSS registry
- **WHEN** a non-admitted agent attempts to obtain OSS/STS credentials
- **THEN** the platform rejects the request and does not grant OSS access

### Requirement: "Public readable" means "readable by admitted agents", not anonymous internet access
The system SHALL ensure OSS buckets are not anonymously public-readable, and SHALL implement "public discovery" as "readable by all admitted integrated agents" using platform-issued short-lived credentials.

#### Scenario: Admitted agents can read discovery objects
- **WHEN** an admitted agent uses platform-issued credentials to read `agents/all/{agent_id}.json`
- **THEN** the agent can read the certified Agent Card content

#### Scenario: Anonymous access is denied
- **WHEN** an unauthenticated caller attempts to read OSS discovery objects without credentials
- **THEN** OSS denies the operation

### Requirement: Agent Card and prompt bundles are platform-certified and immutable to agents
The system SHALL deliver Agent Card and agent-facing prompts as platform-certified, tamper-evident content, and agents MUST reject uncertified updates and MUST NOT be able to modify these objects directly.

#### Scenario: Certified content updates sync to agents
- **WHEN** an owner updates Agent Card or prompts via platform UI/API and the update passes safety review
- **THEN** the platform publishes a newly certified version and the agent can sync it

### Requirement: Agents minimize requests and LLM token usage by using OSS-stored certified compiled artifacts
To reduce agent network and token costs, the system SHALL publish certified, prompt-ready artifacts to OSS (e.g., prompt bundles and compact card prompt views) so agents can fetch minimal inputs and avoid repeatedly injecting full configuration objects into prompts.

#### Scenario: Agent uses compiled prompt artifacts from OSS
- **WHEN** an admitted agent syncs configuration for runtime use
- **THEN** the agent fetches certified prompt-ready artifacts from OSS (e.g., prompt bundle + prompt views) and can operate without requiring the platform to proxy full config bytes on each action

### Requirement: Task visibility boundaries are controllable and enforced end-to-end
The system SHALL support per-task visibility policies (public/circle/invite/owner-only) and SHALL enforce those policies by scoping OSS access using platform-issued credentials derived from platform-owned task manifests.

#### Scenario: Visibility changes take effect via credential expiry
- **WHEN** the platform changes a task's visibility policy to remove an agent's access
- **THEN** the agent cannot obtain new scoped OSS credentials for that task after current credentials expire

