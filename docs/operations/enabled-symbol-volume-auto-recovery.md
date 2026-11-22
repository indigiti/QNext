# Automatic futures-volume feed recovery for enabled symbols

This change addresses the operational workaround of switching an enabled index off
and on when SYN/SYN+ volume stalls. It does **not** toggle an index's enabled
state, restart the synthetic-price engine, or create a new subscription demand.

## What runs automatically

Market Core already discovers and subscribes to each configured active market's
current/next futures contracts. The existing watchdog runs once a minute and,
when spot prices remain usable, asks Upstox to reassert Full mode after a
2-minute active-futures silence or 5-minute absence of positive trade volume.

With the opt-in `QNEXT_FUTURES_AUTO_RENEW=1`, the watchdog may escalate:

1. Reassert Full mode for the configured current/next futures keys.
2. If the *active futures stream itself* stays silent after that successful
   attempt and the 5-minute repair cooldown, enqueue an ordered
   `unsub` → `sub(full)` pair for **only the silent active contract**.
3. Keep the currently enabled market set, index price stream and synthetic
   subscriptions unchanged. Keep the next futures contract attached.
4. Resume observing. Any new active futures frame resets the escalation
   sequence, even if it reports a legitimate zero traded delta.
5. Cap targeted renewal at **two attempts per market session per market**,
   with at least 15 minutes between attempts. Failed writer calls may trigger
   soft retries after a minute, but they cannot bypass the hard-renew cap.

Recovery actions are limited to the existing configured and unexpired
futures key. A full/disconnected outbound command queue causes a safe failure,
not a lone `unsub`. On a reconnect, the desired key set is preserved.
No automatic renewal runs outside the exchange session, when the spot source
is stale/unusable, or when the futures feed is active but volume remains zero.
In that last case, only the existing Full-mode repair is attempted; the
system **cannot prove that zero trades is a broker feed defect**.

## Staging configuration

Set only on the Go Market Core staging service, before restart:

```sh
QNEXT_FUTURES_AUTO_RENEW=1
QNEXT_LIVE_INTEGRITY_MONITOR=1
QNEXT_RAW_CAPTURE=1
```

Configure the desired symbols through QNext's existing `active_markets`
configuration. Renewal does not activate symbols omitted from that set.
This flag is disabled by default in production until staging certification.

Check Admin → Feed Status → Live Observability → **Enabled Symbol Volume Feed**.
For each enabled market it shows the active futures contract, last futures and
positive-volume observations, soft repair attempts, hard renewal attempts and
the latest recovery action. The underlying `feed-status` telemetry also
exposes `auto_renew_enabled`, `renew_attempts`, `last_renew_at_ms`,
`silent_reassertions`, `last_repair_failed` and `last_recovery_action`.

## Certification / rollback

- Verify one entire trading session, including the 15:30–15:40 IST
  derivatives window. No synthetic price bars may be reset or rewritten.
- Test a single silent futures key while another enabled market and the
  next futures contract remain healthy; only the affected active key may
  be renewed.
- Test soft repair on quantity silence with live futures quotes: **never**
  force unsub/sub when legitimate zero trade volume is possible.
- Test a disconnected/full command queue: both commands must be rejected
  together, with no partial unsubscribe.
- Confirm that new trade volume comes from authoritative cumulative VTT/LTQ
  deltas and matches ledger, candle and SYN+ history; do not manufacture a
  missing tick or distribute an aggregate correction into fabricated 5s bars.
- Compare throughput, WSS update delivery, candle revisions and recorded
  incident evidence against the previous Market Core build.
- To roll back, unset `QNEXT_FUTURES_AUTO_RENEW` and restart Market Core.
  Existing non-destructive `change_mode` watchdog remains operational.

This is an automatic recovery **attempt**, not a guarantee against upstream
market-data loss, zero traded volume, broker quota errors or browser-side
chart refresh issues. Real incident captures are still needed to prove the
original QNext failure is resolved.
