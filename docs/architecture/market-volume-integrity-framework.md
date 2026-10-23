# QNext market volume integrity framework

Implementation status: ledger/reconciliation implementation complete in code; staging-first live certification required.

## Authority contract

For every configured cash index, the Upstox cash-index provider is an authority for
**price**, not traded index **volume**. QNext represents its futures-volume
indicator as a labelled *futures proxy*. Option-parity SYN/SYN+ prices are
separate derived observations and do not create native traded index volume.

- Historical 1m / provider 1W / provider 1M index candles cannot overwrite
  previously recorded futures-proxy volume. Missing futures data is not reconstructed
  from a cash-index provider's `volume` field.
- Higher timeframe aggregates are constructed from canonical minute history,
  including any verified proxy quantities already present there.
- SYN+ retains independent microprice/midpoint/LTP parity prices and mirrors
  the associated market's canonical source-volume revisions.
- Native basket activity (option-leg LTQ/VTT) is not interchangeable with futures
  volume. Any future unified SYN volume display must retain the original metric
  as a separately labelled indicator.

## Multi-market shadow activation

Market Core and the symbol registry read `QNEXT_SYN_PLUS_ENABLED_MARKETS`
(comma-separated, uppercase-insensitive; default: `NIFTY`). For example,
`NIFTY,BANKNIFTY` enables the two independent SYN+ shadow bridges and volume
source observers. All other configured markets remain disabled until explicitly
activated and feed-certified.

`qnext-data-collector` remains one market per process. Set
`QNEXT_DATA_COLLECTOR_MARKET=BANKNIFTY` on an additional collector instance to
produce BANKNIFTY SYN+ research snapshots. That instance needs its own
`QNEXT_SYN_PLUS_INSTRUMENT_ID=QNEXT:BANKNIFTY-SYN+` and, if configured,
`QNEXT_SYN_PLUS_VERSION`. Keep the existing NIFTY collector unchanged.

Do **not** enable an index whose selected futures contract cannot be discovered
and whose full-mode futures data is not arriving. Do not reuse a NIFTY provider
key for another market. All market state is keyed independently.

## Implemented integrity features (pending live-market certification)

1. Independent file-backed append-only ledger under
   `QNEXT_STORAGE_ROOT/futures-volume-ledger/`, keyed by index and UTC day.
   Each accepted proxy record includes its provider, original futures contract,
   event timestamp, source sequence, cumulative traded volume and quantity.
   The market sink durably appends and fsyncs an observed proxy record before
   incorporating the quantity into a canonical index candle.
2. Explicit 5s/15s/30s/1m recovery from observed source timestamps.
   Historic recovery only corrects *existing* price candles using append-only
   revisions, preserving OHLC, publishing resync and never distributing 1m
   volume proportionally to invent subminute trades.
3. Shared INDEX/SYN/SYN+ futures-volume semantics: SYN live price candles
   mirror their underlying market's canonical futures volume in the WSS
   broker, SYN and SYN+ REST receive the matching source overlay, and the
   SYN+ shadow bridge continues to reconcile its source candle revisions.
   The underlying SYN/SYN+ price algorithms remain unchanged.
4. REST/WSS bars carry `volume_source` and `volume_status` metadata,
   including `OBSERVED`, `UNKNOWN` and `LEGACY_UNVERIFIED`.
   Unavailable futures data is **not** a verified zero trade and is never
   replaced by synthetic basket activity.
5. Opt-in multiple SYN+ collectors keep independent index volume sources;
   NIFTY is still the only default-enabled SYN+ market.

## Operational limits

- The journal starts when this release is deployed. It cannot reconstruct
  subminute trades that were never observed or captured historically.
- A provider outage or missing VTT/LTQ is not repaired by price-history
  candles; missing futures volume remains explicitly unknown.
- An `OBSERVED` label means genuine provider deltas were captured, not that
  upstream reporting is complete. Out-of-order/corrected historical provider
  trades require a separate authoritative futures source to certify totals.
- One fsync per accepted proxy observation prioritizes durability over the
  lowest possible ingest latency; benchmark this under real high-throughput
  conditions before production promotion.
- Do not describe any of this as a certified uninterrupted feed until
  live trading sessions and contract expiry rollovers have been observed.

## Staging rollout gates

- CI: Q0, Q1 full `go test -race ./...`, Q1–Q5, shadow bridge tests,
  and immutable `digiops-release` artifact verification must pass.
- Confirm volume does not change SYN/SYN+ OHLC or virtual symbol identities.
- Compare 15s/30s/1m/5m NIFTY and selected second-index REST/WSS
  volumes and revisions, including finalized candle correction.
- Monitor at least one full live market session before enabling more indices.
- Roll back by clearing `QNEXT_SYN_PLUS_ENABLED_MARKETS` to `NIFTY` or
  deploying the previous immutable DigiOps release.

No component may invent a traded quantity to conceal an upstream feed outage.
