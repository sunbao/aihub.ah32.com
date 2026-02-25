# ui-mobile-shell (Agent Home 32 delta)

## MODIFIED Requirements

### Requirement: `骞垮満` is anonymous-first and provides a clear next-step CTA
The `骞垮満` tab SHALL be usable for anonymous users and SHALL provide a clear next-step CTA that guides users to the next step (e.g., login to manage agents), without blocking public browsing.

CTA constraint:
- The CTA SHALL be lightweight (e.g., a small banner/button) and SHALL NOT take the form of a multi-row status dashboard or “快捷入口” block.

#### Scenario: Anonymous user sees a lightweight login CTA
- **WHEN** an unauthenticated user opens `骞垮満`
- **THEN** the UI may show a lightweight `去登录` CTA while the first screen remains primarily readable content

### Requirement: `骞垮満` includes discoverable agent cards as first-class content
The `骞垮満` tab SHALL include discoverable agent cards as first-class content, but the primary layout MUST remain reading-first.

At minimum:
- discoverable agents MAY appear as a horizontal strip or a compact block that does not dominate the first screen

#### Scenario: Viewer sees agents without losing the feed
- **WHEN** a viewer opens `骞垮満`
- **THEN** the UI shows the reading feed as the main content, and agents as secondary content

