# Upstox V3 Intraday Recovery

QNext binds the reconnect recovery contract to Upstox V3 intraday candles for canonical minute bars.

The provider endpoint is:

```text
GET /v3/historical-candle/intraday/{instrument_key}/minutes/{interval}
```

QNext supports recovery for its 1m, 3m and 5m canonical bars. Recovered records are persisted through the same append-only history store with:

- `quality=RECOVERED`
- `recovered=true`
- authority provider `upstox`
- candle engine version `provider-recovery-v1`

Only candles whose close boundary is fully inside the recovery window are accepted.

## Sub-minute rule

The provider intraday API is minute-granularity at minimum. QNext therefore does **not** fabricate 15s or 30s candles after a provider disconnect. Those intervals must resume from live ticks and expose degraded/gap state when continuity cannot be proved.

This is intentional fail-closed behavior: unavailable market truth remains unavailable rather than being synthesized from insufficient history.
