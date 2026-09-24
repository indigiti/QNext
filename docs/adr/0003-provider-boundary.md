# ADR-0003: Provider adapter boundary

- Status: Accepted
- Date: 2026-09-24

## Decision

Provider-specific protocols terminate inside provider adapters. QNext downstream code consumes canonical instruments, ticks and bars only.

An adapter owns provider authentication, wire protocol, decoding, subscription semantics, reconnect behavior and provider-specific identifiers. Normalization occurs immediately after decoding.

The first Upstox slice targets Market Data Feed V3. Protobuf decoding and WebSocket connectivity will be added behind the same adapter boundary; unit fixtures operate on already-decoded V3 messages.

## Volume semantics

Provider "last traded quantity" is not equivalent to canonical candle volume. Canonical ticks therefore carry separate `last_quantity` and `volume_delta` fields. A provider adapter may emit a non-zero `volume_delta` only when it can derive that delta reliably from provider data. Candle volume aggregates `volume_delta`; it never blindly sums last-traded quantity.

## Consequences

- Provider payload changes cannot leak into Vela, Python or Strategy Lab.
- Provider instrument keys map through the QNext symbol registry.
- The market core can replace or fail over providers without changing canonical identities.
- Tests can certify normalization independently from live credentials and networking.
