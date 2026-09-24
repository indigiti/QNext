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
- **No application database in the initial architecture**
- TypeScript/Node tooling may be used during build time only.
- Durable state and market history use structured file-based persistence.
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

**Q0 — Foundation**

Q0 freezes contracts and engineering boundaries before provider, UI, strategy, or AI implementation.

### Q0 exit gate

- QNext naming is canonical.
- Runtime stack and deployment constraints are frozen.
- Cross-language market contracts are versioned.
- REST/WebSocket contracts are specified.
- Core architecture decisions are recorded as ADRs.
- Certification gates and fixture conventions exist.
- CI validates repository structure and contract files.

## First vertical slice after Q0

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
