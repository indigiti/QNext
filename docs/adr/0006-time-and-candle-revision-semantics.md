# ADR-0006: Time and candle revision semantics

Status: Accepted

## Decision

QNext preserves distinct timestamps for:

- provider/market event time
- QNext receipt time
- QNext processing time
- QNext publication time

All wire timestamps use Unix epoch milliseconds in UTC. Instrument timezones are metadata used for display and session interpretation; they never change the wire-time basis.

## Candle identity

A canonical candle is identified by:

```text
instrument_id + timeframe + open_time_ms
```

A candle is forming while `final=false`. At the canonical close boundary it becomes `final=true`.

## Corrections

A finalized candle may be corrected only through an explicit revision.

- initial emitted candle: `revision=0`
- each correction increments the revision
- `corrected=true`
- provenance identifies recovery/correction origin
- downstream consumers must never silently replace history without observing the revision

## Late and out-of-order ticks

Provider adapters preserve source timing and sequence information. The Market Core decides whether a late tick is discarded, incorporated before finality, or emitted as a historical correction after finality. That policy is versioned and testable.

## No future data

A historical consumer may only observe information available at its simulated timestamp. This is a release-blocking invariant for intelligence and replay.
