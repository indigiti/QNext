# QNext

QNext is a market-intelligence platform built around clear runtime ownership boundaries:

- **PHP** owns the conventional web application/backend surface: application routing, authentication/session integration, permissions, configuration endpoints, workspace persistence endpoints, and administration HTTP flows.
- **TypeScript + Vela** owns the browser application and chart/workspace presentation.
- **Go Market Core** owns realtime market truth, provider connectivity, normalization, synthetic markets, canonical candles, recovery, integrity, subscriptions, and WebSocket fan-out.
- **Python Intelligence** owns feature engineering, statistical/ML intelligence, predictions, outcomes, and research.
- **Strategy Lab** owns deterministic replay, backtesting, shadow execution, paper trading, and performance attribution.

## Runtime constraints

QNext intentionally starts with a lean deployment model:

- **No production Node.js server**
- **No application database during initial live validation**
- TypeScript/Node tooling may be used during build time only.
- Durable state and market history use structured, versioned file-based persistence.
- MariaDB is the planned relational migration target only after at least 30 consecutive days of stable live operation and an explicit migration-readiness review.
- Redis, if introduced, is optional and ephemeral; it is not authoritative storage.

## Production request paths

```text
Browser
  │
  ├── HTTP ───────────────► PHP
  │
  └── Market WebSocket ───► Go Market Core
                                │
                                └── Providers

Go Market Core ── canonical data ──► Python Intelligence
```

PHP is not placed in the realtime market-data path.

## Current phase

**Q1-Q5 plan closure before Q6 Production.**

The consolidated Q1-Q5 CI baseline is green. Remaining detailed-plan closure items are tracked before Q6 production certification.

Persistence remains file-backed/no-DB during this closure and through initial live validation. See ADR-0009 for the later MariaDB migration gate.

## Certified vertical slice

```text
Provider
  ↓
QNext Market Core
  ↓
Canonical Tick
  ↓
Canonical Candle Engine
  ↓
File-backed History + Integrity
  ↓
QNext REST / WebSocket
  ↓
QNextProvider
  ↓
Vela Workspace
```

Initial instruments:

- `QNEXT:NIFTY`
- `QNEXT:NIFTY-SYN`

## Architecture rule

> Go owns realtime truth. Vela owns presentation. Python owns intelligence. Strategy Lab owns research and simulated execution. PHP owns the conventional web application layer.

See `docs/architecture/`, `docs/adr/`, and `schemas/`.
