# Q4 — Strategy Lab

## Purpose

Q4 establishes deterministic replay, backtesting, shadow-compatible execution semantics, and performance attribution on top of canonical QNext market data.

It is intentionally downstream of Market Core.

## Hard boundaries

1. Q4 never connects directly to market-data providers.
2. Q4 never generates or revises canonical candles.
3. Q4 consumes final, quality-eligible canonical bars.
4. Strategy code sees no future bar.
5. Signals are timestamped at the current final bar close.
6. Orders created from a signal fill no earlier than the next bar.
7. Execution assumptions are versioned and included in run identity.
8. Dataset identity is cryptographically hashed.
9. Run artifacts are immutable file-backed records.
10. Strategy results cannot alter live market truth.

## Initial Q4 exit gate

The first Q4 slice is considered established when deterministic replay produces identical run/result hashes for identical inputs; non-final/degraded/duplicate inputs fail closed; same-bar fills are impossible; slippage and fees are explicit; equity, PnL, and maximum drawdown are reproducible; immutable file-backed output is available; and CI certifies Strategy Lab independently.

## Deferred within Q4

Follow-on slices include order types beyond deterministic market execution, multi-instrument portfolios, exchange-session calendar integration, shadow/live-paper event adapters, richer trade excursion attribution, parameter sweep orchestration, walk-forward validation, and benchmark-relative attribution.

These must be added without weakening the anti-look-ahead and immutability contracts.
