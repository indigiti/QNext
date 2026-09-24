# ADR-0001: Go owns realtime market truth

Status: Accepted

## Context

QNext needs one authoritative realtime path for provider connectivity, normalization, synthetic calculation, candle construction, recovery, integrity, subscription management, and browser fan-out.

## Decision

The Go Market Core is the authoritative realtime/data-plane service.

Python is an asynchronous consumer and must never sit in the critical chart-data path. Vela consumes QNext APIs and streams; it never connects directly to brokers/providers.

## Consequences

- Canonical ticks and bars are produced once.
- All downstream consumers share the same market truth.
- Market-data delivery remains independent from intelligence failures.
