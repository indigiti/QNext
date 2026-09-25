# Q6 — NIFTY-SYN Auto Leg Manager

Status: **CI CANDIDATE**

QNext NIFTY-SYN no longer needs a fixed list of option instrument keys. The Market Core can maintain an expiry-aware option basket automatically from the live NIFTY reference price.

## Default policy

- strike interval: 50 points
- active strikes: 5
- active window: ATM-100, ATM-50, ATM, ATM+50, ATM+100
- warm strikes: 7, keeping one additional strike ready on each side
- minimum valid parity candidates: 3
- ATM hysteresis: 5 points
- ATM confirmation: 750 ms
- expiry policy: nearest non-expired expiry with a complete warm CE/PE ring
- calculation: median of valid K + CE - PE candidates
- leg freshness and CE/PE event-time skew remain mandatory quality gates

## Runtime sequence

1. QNext subscribes to the NIFTY underlying.
2. The manager derives the nearest 50-point ATM.
3. It resolves the 7-strike CE/PE warm ring from the Upstox Option Contracts API.
4. Dynamic provider mappings are registered with expiry-aware canonical option IDs.
5. Missing warm legs are subscribed on the existing Upstox WebSocket session.
6. The current NIFTY-SYN generation continues to publish while the pending generation warms.
7. The pending generation becomes authoritative only after it produces a GOOD synthetic observation.
8. Obsolete outer contracts are unsubscribed after the atomic switch.
9. The desired subscription snapshot is reused after reconnect.

No synthetic observation is allowed to mix expiries.

## Hysteresis

For a current ATM of 25,100 with 50-point intervals and 5-point hysteresis:

- the mathematical upper boundary is 25,125;
- QNext rolls upward only above 25,130;
- the mathematical lower boundary is 25,075;
- QNext rolls downward only below 25,070;
- the new ATM must remain selected for the configured confirmation period.

This prevents rapid 25-point-boundary oscillation from causing subscription churn.

## Expiry handling

The resolver examines non-expired Upstox option contracts in expiry order. An expiry is eligible only when every requested warm strike has both a CE and PE. If the nearest expiry is incomplete, QNext advances to the next complete expiry rather than mixing expiries.

Canonical option IDs include underlying, expiry, strike and side, for example:

`NSE:NIFTY:2026-09-29:25100:CE`

## Resilience

When auto-leg mode is enabled:

- Upstox/Dhan authority failover continues to govern the NIFTY underlying.
- Dynamic option legs are currently Upstox-only inputs to NIFTY-SYN.
- Option ticks therefore bypass the multi-provider authority router and feed only the synthetic assembler.
- If the Upstox option basket is unavailable or stale, NIFTY-SYN stops advancing rather than fabricating a value.
- A later Dhan option-leg implementation can add secondary option authority without changing the NIFTY-SYN domain contract.

## Pricing scope

This slice intentionally retains the certified LTPC decoder and the existing robust median calculation. Bid/ask midpoint or liquidity-weighted pricing requires a separately certified full-depth decoder and is not silently mixed into this change.

## Rollback

The legacy fixed 10-leg configuration remains supported. Removing the `synthetic.auto` block and supplying the original ten fixed CE/PE legs returns QNext to the previous deterministic mode.


## Certification

PR merge requires the Q6 Auto Leg Manager gate plus the inherited Q0/Q1/Q2 and Q1-Q5 regression gates to pass on the final non-bot branch head.
