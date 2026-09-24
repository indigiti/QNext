# QNext Market Core

Q1 establishes the authoritative realtime market-data service.

## Q1 scope

The first vertical slice is deliberately narrow:

```text
Provider adapter
    ↓
Canonical Tick
    ↓
Candle Engine
    ├── 15s
    ├── 30s
    ├── 1m
    ├── 3m
    └── 5m
    ↓
History / integrity boundary
    ↓
REST + WebSocket delivery
```

Initial target instruments:

- `QNEXT:NIFTY`
- `QNEXT:NIFTY-SYN`

This service contains no PHP, Node.js server, Vela rendering, or Python inference code.

## Current implementation

- canonical in-memory market domain types
- deterministic candle engine
- versioned synthetic calculation with quality gating
- provider-port skeleton
- health/readiness/version HTTP endpoints
- unit tests for candle and NIFTY-SYN parity

Provider connectivity and persistent file-backed history are added incrementally behind these boundaries.
