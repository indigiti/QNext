# Q4 — Strategy Lab

## Purpose

Q4 establishes deterministic replay, backtesting, paper execution, chart execution markers, and performance attribution on top of canonical QNext market data.

It is intentionally downstream of Market Core.

## Hard boundaries

1. Q4 never connects directly to market-data providers.
2. Q4 never generates or revises canonical candles.
3. Q4 consumes final, quality-eligible canonical bars.
4. Strategy code sees no future bar.
5. Signals are timestamped at the current final bar close.
6. A bar-close signal cannot use future bar data; the reference execution model fills at the next bar open.
7. Replay and paper use the same execution engine and incremental state transition path.
8. Execution assumptions are versioned and included in replay identity.
9. Dataset identity is cryptographically hashed.
10. Replay artifacts are immutable file-backed records.
11. Every simulated fill produces a canonical chart marker.
12. Strategy results cannot alter live market truth.

## Q4 exit gate

Q4 is complete only when all four project requirements are certified:

| Gate | Required evidence |
| --- | --- |
| deterministic replay + backtesting | identical replay inputs/configuration produce identical run and result hashes |
| paper engine | canonical final bars can be processed incrementally with deterministic orders, fills, positions and equity |
| chart markers | every fill emits a stable BUY/SELL marker consumable by the chart layer |
| replay/paper parity PASS | identical bars + strategy + execution configuration produce identical fills, equity curve, chart markers, costs, PnL and drawdown |

The parity gate is executable in `services/strategy-lab/tests/test_engine.py::BacktestEngineTest.test_replay_paper_parity`.

## Architecture

```text
                    Canonical final bars
                           │
                 ┌─────────┴─────────┐
                 │                   │
          finite replay         streaming paper
                 │                   │
                 └─────────┬─────────┘
                           ▼
                     PaperEngine
                           │
                  shared execution model
                           │
               ┌───────────┼───────────┐
               ▼           ▼           ▼
             fills       equity      markers
```

The replay engine is a deterministic finite driver over `PaperEngine`; it does not implement a second fill algorithm.

## Deferred beyond the Q4 exit gate

Follow-on work may add additional order types, multi-instrument portfolios, exchange-session calendar integration, live event transport/adapters, richer trade excursion attribution, parameter sweeps, walk-forward validation, and benchmark-relative attribution.

These must not weaken the anti-look-ahead, shared-execution, parity, or immutability contracts.
