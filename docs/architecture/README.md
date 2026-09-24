# QNext Architecture Baseline

QNext preserves one canonical market-data path for Vela, intelligence, replay, backtest, and paper trading while keeping the conventional web application separate from the realtime plane.

```text
                          External Providers
                                  │
                                  ▼
                        QNext Market Core — Go
                                  │
                  ┌───────────────┼────────────────┐
                  │               │                │
            REST / History   WebSocket Fan-out  Canonical Data
                  │               │                │
                  │               ▼                ▼
                  │        TypeScript + Vela     Python
                  │          Browser App       Intelligence
                  │
                  ▼
                 PHP
          Web Application Layer
```

## Runtime ownership

### PHP

PHP owns normal application/backend HTTP concerns:

- application routing
- authentication/session integration
- permissions
- configuration endpoints
- workspace/user preference persistence endpoints
- admin HTTP flows
- release/bootstrap configuration

PHP does not process provider tick streams or browser realtime market fan-out.

### TypeScript + Vela

TypeScript is the browser application language. Vela owns charts/workspaces, indicators, drawings, layouts, multi-chart interaction, themes, persistence surfaces, and chart UX.

Node.js may be used as build tooling for TypeScript/Vela, but there is **no production Node.js application server**.

### Go

Go owns:

- provider connections and reconnects
- unified symbol registry
- feed authority and failover
- normalization and deduplication
- synthetic calculation
- canonical candle generation
- history/recovery/integrity
- subscription management
- browser WebSocket fan-out
- realtime telemetry

### Python

Python consumes canonical QNext data asynchronously for:

- feature engineering
- statistical analysis
- regime detection
- ML/AI
- prediction
- outcome attribution
- research/retraining

Python failure must not interrupt chart data.

## Persistence baseline

QNext starts with **no application database**.

Durable information is stored in structured files outside the public web root. Suggested logical areas are:

```text
private_html/qnext/
├── config/
├── runtime/
├── storage/
│   ├── market/
│   ├── synthetic/
│   ├── replay/
│   ├── predictions/
│   ├── outcomes/
│   └── paper/
└── logs/
```

Formats may include JSON/JSONL for configuration and event/state records and Parquet or other compact analytical files where Python research benefits from them.

The filesystem layout and file formats are versioned contracts. "No database" does not mean "no durable storage."

Redis is optional and may only be used for ephemeral cache, latest-state, session support, or coordination. It is not authoritative historical storage.

## Frozen rules

1. Go owns realtime market truth.
2. Vela owns chart/workspace presentation.
3. PHP owns the conventional web application/backend HTTP layer.
4. Python owns data and AI intelligence.
5. Strategy Lab owns deterministic testing and simulated execution.
6. Browsers never connect directly to broker/provider market feeds.
7. PHP is not in the critical realtime market-data path.
8. There is no production Node.js server.
9. QNext starts without an application database.
10. One canonical candle authority serves every downstream consumer.
11. Provider authority is deterministic, server-side, and auditable.
12. Python failure must not interrupt chart data.
13. Predictions are immutable and outcome-attributed.
14. Self-learning produces validated candidate models; production does not mutate silently.
15. No future information may enter historical features or predictions.
16. Important data/model/strategy/execution definitions are versioned.
17. Production migration requires parity, shadow certification, and rollback readiness.

## Q0 scope

Q0 defines contracts, naming, runtime boundaries, ADRs, fixture conventions, API/WS schemas, persistence conventions, and certification gates. It does not implement provider connectivity, Vela UI, strategy execution, or AI.
