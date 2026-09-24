# QNext Architecture Baseline

QNext preserves one canonical market-data path for Vela, intelligence, replay, backtest, and paper trading.

```text
External Providers
      │
      ▼
QNext Market Core — Go
      │
      ├── Provider Manager
      ├── Connection Manager
      ├── Unified Symbol Registry
      ├── Feed Authority
      ├── Normalizer / Deduplication
      ├── Synthetic Engine
      ├── Candle Engine
      ├── History / Recovery / Integrity
      ├── Subscription Manager
      └── WebSocket Fan-out
      │
      ▼
Canonical Data Path
  ┌───────┼────────────┐
  ▼       ▼            ▼
Gateway  Python     Strategy Lab
  │
  ▼
QNextProvider
  │
  ▼
Vela Workspace
```

## Frozen rules

1. Go owns realtime market truth.
2. Vela owns chart/workspace presentation.
3. Python owns data and AI intelligence.
4. Strategy Lab owns deterministic testing and simulated execution.
5. Browsers never connect directly to broker/provider market feeds.
6. One canonical candle authority serves every downstream consumer.
7. Provider authority is deterministic, server-side, and auditable.
8. Python failure must not interrupt chart data.
9. Predictions are immutable and outcome-attributed.
10. Self-learning produces validated candidate models; production does not mutate silently.
11. No future information may enter historical features or predictions.
12. Important data/model/strategy/execution definitions are versioned.
13. Production migration requires parity, shadow certification, and rollback readiness.

## Q0 scope

Q0 defines contracts, naming, ADRs, fixture conventions, API/WS schemas, and certification gates. It does not implement provider connectivity, Vela UI, strategy execution, or AI.
