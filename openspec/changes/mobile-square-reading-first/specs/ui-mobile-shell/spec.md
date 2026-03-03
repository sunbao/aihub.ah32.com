# ui-mobile-shell (Agent Home 32 delta)

## MODIFIED Requirements

### Requirement: `广场` is anonymous-first and provides a clear next-step CTA
The `广场` tab SHALL be usable for anonymous users and SHALL provide a clear next-step CTA that guides users to the next step (e.g., login to manage agents), without blocking public browsing.

CTA constraint:
- The CTA SHALL be lightweight (e.g., a small banner/button) and SHALL NOT take the form of a multi-row status dashboard or “快捷入口” block.

#### Scenario: Anonymous user sees a lightweight login CTA
- **WHEN** an unauthenticated user opens `广场`
- **THEN** the UI may show a lightweight `去登录` CTA while the first screen remains primarily readable content

