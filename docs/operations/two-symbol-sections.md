# QNext symbol directory: two independent sections

## Intended UI

The QNext chart symbol picker and Admin → Market Data use the same two
*logical* sections. The native Vela symbol picker presents grouped,
expandable roots; the lightweight chart picker uses native HTML optgroups.

### Section 1: Indices & SYN / SYN+

Order is fixed and independent of provider response order:

1. NIFTY
2. NIFTY-SYN
3. NIFTY-SYN+
4. BANKNIFTY
5. BANKNIFTY-SYN
6. MIDCPNIFTY
7. MIDCPNIFTY-SYN
8. FINNIFTY
9. FINNIFTY-SYN
10. SENSEX
11. SENSEX-SYN
12. BANKEX
13. BANKEX-SYN

Only symbols returned by the actual authenticated QNext catalog are chart
selectable. Disabled index markets or opt-in SYN+ shadow symbols must **not**
be fabricated. The admin directory documents the complete intended
catalog with an explicit note that it does not assert feed availability.

All existing index/SYN+ tick, synthetic, derivatives and volume authority
remain unchanged.

### Section 2: Other NSE Symbols · 1m history

Only *additional*, non-synthetic NSE stock/index instruments returned
by the existing `/api/v1/symbols` endpoint are listed. These are the
explicit, capped `nse_1m_symbols` watchlist registered by QNext Market Core
when `QNEXT_NSE_1M_SYNC=1` is enabled. They use completed historical
1-minute bars refreshed periodically (5 minutes), **not live ticks**.

- NSE equities retain official historical traded volume.
- Additional cash indices are price-only; no synthetic/futures proxy
  volume can be copied to the new history-only list.
- Lightweight chart auto-selects 1m for this section and prevents
  changing its interval to a missing 5s/15s/5m series.
- Vela's native picker group title and descriptor note identify the data
  as "1m historical refresh". Vela's chart interval is still under Vela's
  control; if the chart is on another interval, the user must select 1m.
  Do not substitute 1m data for a different requested timeframe.

The Admin Market Data page contains **two separate panels**: a
deterministic ordered core catalog, and actual `nse_1m` Market Core
feed status. The latter displays registered symbols, last successful
refresh, provider error and processed historical bar counts. It does
not falsely claim a symbol is streaming live.

## Implementation boundaries

- No new subscription, price normalization, historical writer or
  backend market configuration API.
- No changes to options legs, futures volume, SYN+, canonical 5-second
  data, Pine strategy calculations or the chart storage format.
- No additional HTTP requests from the Admin console; it uses the
  existing authenticated feed-status refresh.
- Vela's supported `group` + `default` symbol descriptor convention
  is used, with a non-tradable root per available section.
- The root is not returned by QNext Market Core and cannot be resolved
  through the canonical provider registry.
- No more than one group for the new 1-minute universe; empty groups
  do not appear in Vela symbol search.

## Staging acceptance

Verify both chart engines and the Admin Market Data page at 1440,
1024, 390 px. Test:

1. All 13 requested core names are shown in correct order when
   available and no missing symbols appear as fictitious tickers.
2. Selecting NIFTY-SYN+ retains the same futures-volume provenance.
3. The user can expand/collapse Vela roots and select a child member;
   picking a root chooses its first actual default instrument.
4. A configured NSE 1m stock displays its own actual historical OHLCV;
   switch to a core symbol and its previously selected timeframe.
5. A disabled NSE 1m watchlist displays honest empty/disabled states
   rather than a live or healthy badge.
6. No additional Upstox WebSocket subscription is created.
7. Light/dark contrast, long ticker names, small-screen picker behavior,
   keyboard navigation and chart-symbol persistence work.

A successful GitHub build does not certify interactive Vela grouping.
The staging browser check is required before production use.

## Shared multi-exchange browser

The new symbol browser is the primary picker for Lightweight and an additional Vela toolbar action plus Ctrl/Cmd+K. It reads the canonical catalog and builds exchange, asset class and product facets from registered instruments rather than frontend exchange lists. New provider integrations must register canonical identities and explicit `market_category`, `product_type`, and `data_mode` where applicable. A configured stream is not proof of actual live ticks. Lightweight no longer constructs a second full-catalog dropdown. Its newly created panes use canonical IDs and preserve the 1-minute-only data-mode restriction. Vela's existing native selector is retained as a compatibility entry point pending browser verification. This browser does not register any feed or market subscriptions; a user selection opens the chart through the existing provider. Admin configuration and live health stay separate.
