# QNext: selected NSE 1-minute historical symbols (staging opt-in)

This phase adds **explicitly selected NSE equity and index 1-minute
historical candles** without changing index/SYN+ calculations or requesting
any new live Upstox WebSocket subscriptions.

## Configuration (staging Market Core)

Add a small watchlist to the existing JSON file pointed to by
`QNEXT_MARKET_CONFIG`. Preserve all existing `markets`, `active_markets`,
`timeframes` and synthetic settings:

```json
{
  "nse_1m_symbols": [
    {
      "symbol": "RELIANCE",
      "name": "Reliance Industries",
      "provider_key": "NSE_EQ|INE002A01018"
    }
  ]
}
```

The snippet is a **property** to merge into the existing config, not a
replacement configuration file. Resolve each provider key from the current
official Upstox instrument master. Do not infer a key from a ticker name.
For an additional index, use a real `NSE_INDEX|...` key that is **not
already configured as an underlying market**.

Then set `QNEXT_NSE_1M_SYNC=1` on the **Go Market Core staging service**
and restart using QNext's existing deployment controls. With the switch
off, this feature does not register or fetch watchlist instruments. Limit:
**12** distinct symbols per instance. Duplicated canonical instrument IDs,
provider keys, BSE keys, unknown segments and malformed tickers are rejected
at configuration validation. Existing index symbols remain configured
through `active_markets` and should not be duplicated here.

## Behavior

- Each selected symbol appears in QNext `/api/v1/symbols` and can be
  requested via the existing `/api/v1/bars?instrument_id=NSE:RELIANCE&timeframe=1m&...`
  path. No new unauthenticated configuration or fetch endpoint is introduced.
- Market Core fetches Upstox V3 **completed 1-minute candles** for the
  current and previous two IST calendar days at startup and every
  **5 minutes** thereafter, with a 30-second availability buffer.
  Data is persisted in QNext's normal canonical history store with
  revision-safe correction semantics; the broker sends a resync when
  stored bars change. A scheduled poll is **not a live tick feed**, and
  data may lag or be missing if Upstox is slow or unavailable.
- NSE equities preserve Upstox's historical traded volume. NSE cash
  indices use provider OHLC as a **price-only** source; index historical
  volume is **not** treated as SYN+ futures volume.
- Current and future candles are never synthesized. 5-second, 15-second,
  30-second, and other timeframes are **not** fabricated for these symbols.
- `/api/v1/feed-status` includes `nse_1m` with mode,
  refresh interval, configured symbols, last attempt/success, source,
  missing/corrected/unchanged counts, and fetch error diagnostics.
  Absence of an error does **not** certify exchange data completeness.

## Architecture and release safety

- Existing Market Core live subscriptions and `ProviderKeys()` are
  unchanged; no extra futures/option-leg/OI/volume-proxy demand.
- Existing QNext `QNEXT_RAW_CAPTURE`, historical 1m index recovery,
  5s canonical pipeline and chart Pine engines are untouched.
- The new historical poller is disabled by default. There are no
  migrations and rollback is achieved by removing the flag and
  restarting Market Core.
- Not a substitute for a **true low-latency live stock 1-minute
  chart**, which needs a separate explicitly budgeted broker
  subscription/stream design and verification against provider history.

## Acceptance before production enablement

1. Inspect the current Upstox instrument-master key and API permissions for
   each chosen NSE stock/index. Use a small staging watchlist.
2. Verify completed 1-minute OHLC and stock volume against Upstox V3,
   including the 09:15 opening, 15:30 close, weekends and holidays.
3. Confirm no NSE stock key is added to the live WebSocket subscription
   and existing NIFTY SYN+ futures volume stays unchanged.
4. Check initial load, a repeat unchanged poll, a corrected provider
   candle and provider error; no duplicate revisions or invented volume.
5. Review provider quota usage, request latency and Market Core disk
   growth during one full trading session. Do not claim "real-time"
   based on five-minute polling.
6. Verify the new symbols are visible in the chart's search/bars API.
   The trading workspace may still need a follow-up symbol picker
   adjustment before a user can select them conveniently.

Do not automatically enable all NSE-listed stocks. Their number, Upstox
API quotas, exchange corporate actions and storage footprint demand a
separate scoped symbol-catalog and retention design.
