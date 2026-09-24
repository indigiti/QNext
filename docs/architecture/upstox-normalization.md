# Upstox V3 Normalization Boundary

QNext treats broker/provider payloads as external data. Provider-specific keys and timestamp fields are resolved before ticks enter the canonical market pipeline.

## LTPC mapping

For decoded Upstox V3 LTPC envelopes:

- feed map key -> resolved through the QNext symbol registry
- `ltp` -> canonical tick price
- `ltt` -> canonical event time
- `currentTs` -> canonical received/provider-envelope time
- QNext clock -> processed time
- QNext sequence generator -> canonical sequence
- quality -> `GOOD` for a structurally valid normalized tick

Provider feed keys do not become canonical instrument IDs.

## Quantity semantics

Upstox `ltq` is last-traded quantity. It is validated by the adapter, but QNext does not currently map it to canonical bar `volume`.

That is deliberate: last-traded quantity is not automatically equivalent to cumulative traded volume or a safe per-tick volume delta. A separate volume policy must be specified and tested before candle volume is populated from provider fields.

## Determinism

Feed-map keys are sorted before normalization so sequence assignment is deterministic for a decoded multi-instrument envelope.
