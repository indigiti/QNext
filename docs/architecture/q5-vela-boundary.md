# Q5 — Vela / Browser Integration Boundary

Q5 activates the presentation ownership boundary frozen in ADR-0004.

```text
QNext Market Core
  ├── REST /api/v1/bars ───────┐
  └── WS /api/v1/stream ───────┼──► QNextProvider ─► Vela workspace
                               │
PHP application shell ─────────┘
```

## Invariants

1. Vela and browser code consume canonical QNext data only.
2. Browser code contains no broker/provider credentials.
3. Browser code does not implement provider authority, synthetic calculations, candle authority, AI training, or paper accounting.
4. Historical and live bars share one canonical identity: instrument + timeframe + open time.
5. A higher revision may correct an existing bar; stale lower revisions never overwrite newer state.
6. A finalized bar is stable at the same revision.
7. Market-data sequence gaps are not guessed through; the client resynchronizes from canonical REST history.
8. Reconnect uses `RESUME` when a valid stream cursor exists.
9. `RESYNC_REQUIRED` causes a fresh REST/history boundary and a fresh subscription.
10. Node.js is build/test tooling only. Production deploys static browser assets and has no Node application server.

## Integration surface

The Q5 package exposes a thin `QNextProvider`. Vela can bind its chart series to provider `bars` events and provider status to connection UI. No Vela-specific fork is necessary in this phase.
