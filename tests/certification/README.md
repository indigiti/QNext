# QNext Certification Gates

These gates are release blockers where applicable.

## Market truth

- MARKET_DATA_PARITY
- SYNTHETIC_PARITY
- CANDLE_PARITY
- HISTORY_LIVE_CONTINUITY
- NO_DUPLICATE_TICKS
- NO_GAP
- PROVIDER_FAILOVER

## Intelligence safety

- NO_FUTURE_DATA
- FEATURE_PARITY
- PYTHON_FAILURE_ISOLATION

## Strategy/replay

- BACKTEST_DETERMINISM
- PAPER_REPLAY_PARITY

## Runtime

- WS_MULTI_CLIENT
- VELA_RUNTIME
- MOBILE_RUNTIME

Each gate must eventually have:

1. deterministic fixture input,
2. expected output,
3. machine-readable pass/fail result,
4. artifact/version identity.
