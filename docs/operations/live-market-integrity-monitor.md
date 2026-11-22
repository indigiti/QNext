# Live market integrity monitor (staging-first)

The QNext Go Market Core now has an opt-in read-only monitor of **published**
canonical and SYN/SYN+ bar events. It adopts Tickpipe-style replay diagnostics
without running Tickpipe, adding a feed connection, or rewriting market logic.

## Enable on an isolated staging Market Core

Set these variables on the **Go Market Core service** before restarting it:

```sh
QNEXT_LIVE_INTEGRITY_MONITOR=1
QNEXT_RAW_CAPTURE=1
QNEXT_STORAGE_ROOT=/path/to/qnext/storage
```

Keep the existing `QNEXT_MARKET_CONFIG` and broker credentials unchanged.
The monitor is disabled by default, including in production, until explicitly
enabled and certified. `QNEXT_RAW_CAPTURE=1` is independent: enable it if a
future incident must be replayable. Confirm available disk space and the existing
30-day raw-frame retention policy. Raw capture is **best-effort**, and frame drops
are reported rather than silently treated as complete evidence.

## Automatic checks

Every published bar for configured cash-index and synthetic aliases on
5s, 15s, 30s or 1m is copied to a bounded (4096-event) channel under the
broker's existing ordering lock. On overflow, live publishing is never blocked:
the monitor tracks `AUDIT_QUEUE_OVERFLOW` and marks its evidence incomplete.

Every three seconds the monitor also checks the last two buckets per stream:
- **SYN_FUTURES_VOLUME_LAG**: a SYN/SYN+ bar is behind an **OBSERVED**
  `FUTURES_PROXY` source candle for the *same bucket* after a 3s grace period.
  No assertion is made when upstream volume is unknown or missing; price
  feeds/option baskets cannot establish true futures volume.
- **PUBLISHED_CANDLE_GAP**: a discontinuity between emitted buckets in the
  same Indian trading date. This is a publication gap, not proof of lost trades.
- **VOLUME_REGRESSION**, **STALE_REVISION_PUBLISHED**, and
  **UNREVISIONED_FINAL**: suspicious published revisions or quantity changes.
- **RAW_CAPTURE_LOSS**, **RAW_CAPTURE_WRITE_ERROR**, and
  **AUDIT_QUEUE_OVERFLOW**: diagnostics that make a recorded replay incomplete.

This initial monitor does **not** compare the browser's rendered markers or
PineWorkerEngine outputs, automatically execute incident replays, backfill
missing broker ticks, or repair production data. No alert triggers a trade.

## Where to check results

- Admin → Feed Status → Live Observability → **LIVE INTEGRITY**.
  Existing feed-status API includes `live_integrity` with `enabled`,
  `state`, `counts`, `recent`, `audit_dropped`, raw-capture counters,
  and journal errors. The panel refreshes every 5 seconds.
- JSONL anomaly records are appended under
  `QNEXT_STORAGE_ROOT/live-integrity/YYYY-MM/YYYY-MM-DD.jsonl`, dated in IST.
  The service log also emits each alert. No externally writable endpoint
  is added. Protect these files as operational market data.

`state=ALERTS_RECORDED` means incidents were detected since this process
started, **not** that they remain unresolved. If no source observation was
available, zero detected alerts does not establish source completeness.
Alert deduplication is per symbol/timeframe/candle/rule within a process day.

## Staging acceptance gates

1. Deploy the PR build to staging only; verify `live_integrity.enabled=true`.
2. Run a complete live trading session including the 15:30–15:40 IST
   derivatives window, with raw capture enabled.
3. Check NIFTY SYN+ 5s/15s/30s/1m under normal price, correction,
   reconnect, and expiry rollover conditions. Confirm alert records and
   counters match the published bar sequence without altering OHLC or volume.
4. Deliberately inject a delayed observed futures-proxy bar in an isolated
   test; expect a lag alert after grace, then the normal revision correction.
5. Test raw capture and audit channel overflow in a load environment; live
   tick handling must not regress. Compare Market Core publish latency with
   the previous staging build.
6. Confirm no browser refresh is needed to see reconciled candle volume;
   independently investigate chart/Pine parity. A server-side monitor cannot
   establish browser-rendered parity by itself.
7. Do not promote to production until tests, live-session certification and
   rollback are verified. Roll back by clearing
   `QNEXT_LIVE_INTEGRITY_MONITOR` and restarting Market Core.

This is detection and evidence collection only, not lossless brokerage tick
recording or an unattended market simulator.
