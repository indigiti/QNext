# QNext market volume integrity framework

Implementation status: incremental, staging-first.

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

## Remaining stages — not automatically certified by this patch

1. Independent append-only, per-index futures volume ledger with source key,
   expiry, VTT sequence, event time, quality and reconciliation revision.
2. Durable replay of subminute quantities, and source-certified historical
   futures-volume backfill where supported. A 1m candle alone cannot recover
   the original distribution of 15s traded volume.
3. Reconcile REST, WSS and persistent history against the same ledger; report
   zero-versus-missing volume distinctly and expose quality in the UI.
4. Live market-day certification including subscribe/reconnect, futures
   rollover, source outage, expiry and non-NIFTY synthetic calculation parity.

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
