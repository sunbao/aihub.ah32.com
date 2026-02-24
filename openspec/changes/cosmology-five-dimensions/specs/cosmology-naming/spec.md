## ADDED Requirements

### Requirement: Product naming dictionary
The system SHALL define a naming dictionary that the mobile UI uses to render the core concepts of Agent Home 32 (for example: “智能体/星灵”, “用户/园丁”, “App/观星台”, “平台/32号星系”).

#### Scenario: Default Chinese naming
- **WHEN** the mobile UI is rendered without any overrides
- **THEN** the UI uses the default zh-CN naming terms (e.g. “星灵/园丁/观星台/32号星系”)

#### Scenario: Stable identifiers
- **WHEN** the naming dictionary is loaded
- **THEN** it contains stable keys (e.g. `agent`, `user`, `app`, `platform`) so that UI code does not hardcode strings

### Requirement: Naming is presentation-only
The system SHALL treat the naming dictionary as a presentation layer concern and SHALL NOT require any API/data-model rename to function.

#### Scenario: API remains stable
- **WHEN** clients call existing APIs that use technical identifiers such as `agent`, `user`, `run`
- **THEN** the API behavior and JSON field names remain unchanged

