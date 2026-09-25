# Q6 Automatic Market Gap Recovery

## Goal

Recover automatically from an Upstox WebSocket interruption or watchdog-triggered stale session without fabricating market data.

## Detection

The Upstox supervisor treats a disconnected or watchdog-expired stream as a provider gap. QNext tracks the last accepted event time independently for every canonical instrument rather than using one global cursor.

This matters when several indices are active: a quiet BANKNIFTY stream must not inherit a later NIFTY recovery cursor.

## Automatic recovery

For every directly subscribed index, QNext requests Upstox intraday historical candles for the recoverable canonical timeframes:

- 1m
- 3m
- 5m

Only fully closed candles inside that instrument's outage window are persisted. Recovered records are marked:

- `quality=RECOVERED`
- `recovered=true`
- `authority_provider=upstox`

After at least one bar is repaired for an instrument/timeframe, Market Core sends a `resync_required` control message to current QNext WebSocket subscribers. The browser reloads canonical REST history and then resumes the live WebSocket stream. Recovered historical bars are therefore not misrepresented as new live ticks.

## 15s / 30s

Upstox's intraday history API used by QNext provides minute candles; it does not provide the original missed tick sequence for a disconnected WebSocket interval. QNext therefore does not fabricate 15s or 30s bars from a 1m candle.

Admin telemetry reports these as non-exact recovery timeframes.

## Exact tick replay

Exact reconstruction of ticks that QNext never received requires one of:

1. a provider-supported historical tick/replay API,
2. a simultaneously captured secondary live provider such as Dhan, or
3. another authoritative tick archive.

Upstox V3 reconnect provides a fresh snapshot and resumes the live stream, but QNext does not treat that snapshot as replay of all missed intermediate ticks.

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
Upstox intraday history
    |
    +--> repair 1m / 3m / 5m canonical history
    |
    v
Broker resync_required
    |
    v
Browser REST history reload
    |
    v
resume QNext WSS live stream
```

## Dhan

Dhan remains disabled in the active runtime. When the deferred Dhan work is activated, the same gap model can be extended so a secondary live source fills provider outages at finer granularity.
