# Q6 Automatic Market Gap Recovery

## Goal

Recover automatically from an Upstox WebSocket interruption or watchdog-triggered stale session without fabricating market data.

## Detection

The Upstox supervisor treats a disconnected or watchdog-expired stream as a provider gap. QNext tracks the last accepted event time independently for every canonical instrument rather than using one global cursor.

This matters when several indices are active: a quiet BANKNIFTY stream must not inherit a later NIFTY recovery cursor.

## Automatic recovery

For every directly subscribed cash index, QNext requests **canonical 1m** Upstox intraday history for the affected outage window. Only fully closed candles are admitted.

Recovered canonical records are marked:

- `quality=RECOVERED`
- `recovered=true`
- `authority_provider=upstox`

The repair is revision-aware: a missing minute is appended as revision 0; a provider minute that differs from an already stored minute is appended as the next higher revision instead of silently overwriting history.

After canonical 1m repair, QNext deterministically rebuilds every enabled minute/hour rollup from repaired 1m history:

- 2m, 3m, 5m, 10m
- 15m, 30m, 45m
- 1h, 2h, 3h, 4h
- a completed 1D candle when a complete certified session exists

A derived candle is written only when all required canonical minutes are present and contiguous. QNext does not fill an incomplete target bucket with guessed OHLC.

After any canonical or derived stream changes, Market Core sends `resync_required` for that instrument/timeframe. The browser reloads canonical REST history and resumes the live WebSocket stream.

## 15s / 30s

15s and 30s are tick-built. Upstox minute history does not contain the original missed tick sequence, so those intervals cannot be exactly reconstructed after an upstream outage. QNext deliberately leaves such holes visible instead of fabricating second candles.

Admin telemetry therefore keeps 15s/30s in `non_exact_timeframes` unless an authoritative secondary tick source or historical tick replay is available.

## Demand-idled synthetics

Synthetic option baskets are demand-aware. When browser/strategy references reach zero and the idle grace expires, QNext unsubscribes the synthetic option wing to reduce provider load. No authoritative synthetic ticks are produced during that idle interval, so a later browser refresh cannot recreate exact synthetic OHLC.

Continuous synthetic history requires a deliberate persistent data-collection or strategy reference. This is a product policy, not a cash-index history-repair failure.

## Exact tick replay

Exact reconstruction of ticks that QNext never received requires one of:

1. a provider-supported historical tick/replay API,
2. a simultaneously captured secondary live provider such as Dhan, or
3. another authoritative tick archive.

Upstox reconnect provides a fresh snapshot and resumes the live stream, but QNext does not treat that snapshot as replay of all missed intermediate ticks.

QNext raw-frame capture can preserve frames that reached Market Core, but it cannot create frames that were absent during an upstream/network outage.

## Runtime flow

```text
Upstox WSS
    |
    | disconnect / watchdog stale
    v
per-instrument gap cursors
    |
    v
Upstox canonical 1m intraday history
    |
    +--> revision-aware 1m repair
    |
    +--> rebuild enabled session-aligned rollups from 1m
    |
    v
Broker resync_required for every changed timeframe
    |
    v
Browser REST history reload
    |
    v
resume QNext WSS live stream
```

## Dhan

When Dhan standby is configured and certified as authoritative for the required instrument/tick semantics, the same gap model can be extended to fill finer-granularity outages. Until then QNext does not label reconstructed seconds as exact.
