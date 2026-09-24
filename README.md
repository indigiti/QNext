# QNext

QNext is a market-intelligence platform built around four strict ownership boundaries:

- **Go Market Core** owns realtime market truth, provider connectivity, normalization, synthetic markets, canonical candles, recovery, integrity, subscriptions, and fan-out.
- **Vela Workspace** owns chart and workspace presentation.
- **Python Intelligence** owns feature engineering, statistical/ML intelligence, predictions, outcomes, and research.
- **Strategy Lab** owns deterministic replay, backtesting, shadow execution, paper trading, and performance attribution.

## Current phase

**Q0 — Foundation**

Q0 freezes contracts and engineering boundaries before provider, UI, strategy, or AI implementation.

### Q0 exit gate

- QNext naming is canonical.
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
History + Integrity
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

> Go owns realtime truth. Vela owns presentation. Python owns intelligence. Strategy Lab owns research and simulated execution.

See `docs/architecture/`, `docs/adr/`, and `schemas/`.
