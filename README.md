# QNext

QNext is a market-intelligence platform built around one authoritative realtime market-data path.

## Architecture

- **Go Market Core** owns realtime market truth, provider connectivity, normalization, synthetic markets, canonical candles, recovery, integrity, subscriptions and WebSocket fan-out.
- **Vela Workspace** owns chart and workspace presentation.
- **Python Intelligence** consumes canonical data asynchronously for reproducible features, models, predictions and outcomes.
- **Strategy Lab** owns replay, backtesting and simulated execution.

## First milestone

Build and certify one canonical **NIFTY / NIFTY-SYN** stream with deterministic timestamps, lineage, candles, history/live continuity, integrity validation and stable REST/WebSocket contracts.

## Development order

1. Q0 — contracts, repository foundation and certification fixtures
2. Q1 — Go Market Core and canonical NIFTY/NIFTY-SYN
3. Q2 — symbol registry and provider authority
4. Q3 — history, recovery and Data Gateway
5. Q4 — Vela provider/workspace
6. Q5 — Strategy Lab
7. Q6 — Python Intelligence and controlled model lifecycle

## Core rules

- Browsers never connect directly to market-data providers.
- One canonical data path serves charts, research, replay, backtests and paper trading.
- Python is never in the critical chart-data path.
- Provider authority is deterministic and auditable.
- Important data, model, strategy and execution definitions are versioned.
- No future information may enter a historical feature or prediction snapshot.
