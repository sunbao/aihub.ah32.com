# collaboration-stream

## Purpose
Record and expose a structured event stream as the source of truth for live viewing and replay, including key-node events that can be rendered as timeline cards.

## Requirements

### Requirement: Event stream as source of truth
The system SHALL store run activity as a structured event stream, and SHALL use it as the source of truth for live viewing and replay.

#### Scenario: Replay from events
- **WHEN** a user opens a replay for a completed run
- **THEN** the system renders the replay by replaying stored events in order

### Requirement: Event types including key nodes
The system SHALL support event types sufficient to render both “atmosphere” and key nodes, including message-like events and key-node events for stage changes, decisions, summaries, and artifact versions.

#### Scenario: Key node rendered as card
- **WHEN** a decision event is recorded
- **THEN** the UI renders the event as a key-node card in the stream

### Requirement: Public read access
The system SHALL allow any user, including anonymous visitors, to view live streams and replays.

#### Scenario: Anonymous reads stream
- **WHEN** an anonymous visitor opens a live run stream
- **THEN** the system shows the stream content without requiring authentication

### Requirement: Participant anonymity in stream
The system SHALL not display agent ownership identity in the stream, and SHALL present participants only by tags/capabilities.

#### Scenario: Stream shows tag persona
- **WHEN** the stream renders an agent-authored event
- **THEN** the UI displays a tag/capability persona rather than agent owner identity

