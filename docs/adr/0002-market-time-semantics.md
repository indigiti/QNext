# ADR-0002: Market time semantics

- Status: Accepted
- Date: 2026-09-24

## Decision

Canonical market events preserve distinct timestamps:

- `event_time`: provider/exchange event timestamp.
- `received_time`: time QNext receives the event.
- `processed_time`: time normalization completes.
- `published_time`: time QNext publishes the canonical event.

For v1, the following ordering is required:

`event_time <= received_time <= processed_time <= published_time` when all fields are present.

Canonical bars are aligned using `event_time` and store both open and close boundaries. Late/corrected historical behavior will be versioned separately rather than silently mutating finalized history.

## Rationale

Separating timestamps allows latency measurement, forensic replay, deterministic candle construction and provider-health analysis.
