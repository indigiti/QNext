# QNeXT OKX XAUUSD X-Perp — staging and deployment gate

## Status
Feature branch `feature/okx-xauusd-marketdata-20261123`, draft PR #171.
Opt-in: `QNEXT_OKX_XAUUSD=1`. **Do not enable in production until staging sign-off**.

## Confirmed against public OKX API on 2026-11-23
- FUTURES instrument `XAU-USD_UM_XPERP-310502`, `ctVal=0.001`, `ctValCcy=XAU`, state=live.
- Public business WebSocket `trades-all` subscription acknowledgment and a real trade.
- Historical finalized 1m candles, and contiguous 1s -> 5s reconstruction.
- Source is a five-year **USD-denominated gold X-Perp**, not retail OTC/MT5 spot XAUUSD.
- Contract metadata is always discovered, never hardcoded as a monthly futures fallback.

## Deployment prerequisites
1. Green GitHub Actions Q0, Q1, Q2, Q1–Q5, Q6 on final commit.
2. Green OKX external preflight `.github/workflows/okx-xauusd-live-preflight.yml` on final code.
3. Stage deploy with `QNEXT_OKX_XAUUSD=1`, first leaving existing NSE/BSE markets unchanged.
4. Inspect `/api/v1/symbols` for `OKX:XAUUSD-XPERP` and Vela Gold & Commodities grouping.
5. Confirm full UTC market availability including weekend and midnight; no NSE holiday filter.
6. Compare live finalized 1m OHLCV with REST-authoritative candles over at least 30 minutes.
7. Interrupt the WS and verify reconnect, no duplicate trade IDs, correct history revisions and
   resync notifications; do not fill missing 5s history with synthetic or 1m-derived OHLC.
8. Confirm 5s/15s/30s/1m/2m/5m and volume display in both QNeXT chart engines.
9. Inspect a live gold feed health sample and exchange-region accessibility on the actual hosting server.
10. Test rollback: set `QNEXT_OKX_XAUUSD=0` and restart market core.
11. Check existing NIFTY-SYN+ and NSE/BSE markets before approving promotion.

## Data integrity notes
- Trade size is **contracts**. Convert using the discovered `ctVal` to troy ounces of XAU.
- REST `vol` is also contracts; converted gold quantity must match trade aggregation where all
  executions have been received.
- On reconnect, revised REST final bars supersede locally formed older revisions. The browser
  receives a resync marker instead of an out-of-order historical live bar stream.
- No automatic failover from XAUUSD X-Perp to XAU-USDT-SWAP.
- Historical 5s repair is conditional on all five confirmed constituent 1s candles being available.
- A passing public API preflight cannot replace a staging browser check or prove zero tick loss.

## Rollback
Revert the feature flag to 0 and restart the service (or roll back the release artifact).
This prevents new OKX subscriptions without altering NSE/BSE processing or stored gold history.

## Evidence
- PR: https://github.com/indigiti/QNext/pull/171
- Live API preflight (success): https://github.com/indigiti/QNext/actions/runs/45219206988

## Admin dashboard feature switch (included in this release candidate)
- Navigate to **Admin → Market Data → OKX XAUUSD X-Perpetual feed**.
- The switch displays the *configured* ON/OFF state separately from actual Market Core status
  (LIVE, WAITING/STALE, disabled, or unsupported release).
- Toggle ON/OFF, click **Save & restart Market Core**, and accept the restart warning.
  The authenticated `GET/PUT /qnext/admin/api/index.php?route=/okx-xauusd` endpoints
  validate a boolean and inherit the existing session/CSRF protections.
- The Ops API stores `{"enabled":true|false}` at the private `config/okx-xauusd.json`
  path, then requests a supervised direct/Cloudways-cron restart. This changes *no secrets*
  and never registers a broker trading account.
- Market Core loads this file at startup beside `QNEXT_MARKET_CONFIG`; when the file exists
  it takes priority over legacy `QNEXT_OKX_XAUUSD`. If absent, the environment flag remains
  backward compatible.
- The switch fails closed on an old Market Core that does not report `okx_xauusd.available`.
  On release #141 (`fd86a1c192c5`) a config change alone cannot install OKX support.
- Because the controller restarts Market Core, existing NSE/BSE streams briefly reconnect.
  Verify NIFTY/SYN+ integrity and OKX trade activity after using the control.
- The status comes from received OKX ticks (not merely configuration): successful restart
  or a checked box is never presented as proof of live data.
