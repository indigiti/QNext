# Q6 Candle Gap Audit

## Scope

This audit separates four conditions that can appear as a visual candle gap in QNext.

1. **Authenticated Admin unavailable** — the Ops API can return HTTP 403 while the public read-only telemetry path remains healthy. This is an Admin-token/session problem, not a Market Core feed failure.
2. **Recoverable cash-index history gap** — Upstox reconnect recovery restores canonical 1m history. Enabled derived minute/hour/day candles must then be rebuilt from the repaired canonical 1m series and subscribers resynchronized.
3. **Unrecoverable second-candle gap** — 15s/30s candles are tick-built. Upstox minute history cannot reconstruct the original missed tick sequence, so QNext must not fabricate exact second candles.
4. **Demand-idled synthetic interval** — synthetic option baskets are activated only while browser/strategy demand exists. When the demand registry reaches zero references and the idle grace expires, no authoritative synthetic ticks are produced for that interval. A later browser refresh cannot recreate exact synthetic OHLC.

## Recovery contract

For active cash indices:

```text
provider disconnect/watchdog
        |
        v
Upstox intraday 1m recovery
        |
        v
revision-aware canonical 1m repair
        |
        +--> rebuild enabled 2m/3m/5m/.../4h from canonical 1m
        +--> rebuild a completed 1D candle when a complete session exists
        |
        v
resync_required for every changed timeframe
        |
        v
browser reloads canonical REST history and resumes WSS
```

The canonical 1m provider record is the recovery authority. Derived bars are exact *at their candle granularity* only when every required canonical minute is present. `aggregateMinuteBars` deliberately refuses to synthesize a derived candle from an incomplete set of minutes.

## Seconds

15s and 30s remain explicitly non-exact after an upstream outage unless QNext has an authoritative secondary tick source or provider tick replay. Gaps must remain visible rather than be filled with invented OHLC.

## Synthetics

Demand-aware subscriptions intentionally trade continuous synthetic history for lower option-feed load. If continuous synthetic history is required for a symbol, a strategy/data-collection reference must keep that synthetic active. This policy must be explicit in telemetry; it is not a history-repair failure.

## Admin authentication

The persisted hashed token in `private_html/qnext/secrets/ops-auth.json` and the legacy `QNEXT_OPS_ADMIN_TOKEN` environment variable can coexist during migration. Authentication checks the persisted hash first and keeps the legacy environment token as a migration fallback. Hosts should remove the legacy environment variable after validating the persisted credential.
