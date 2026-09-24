# QNext Strategy Lab — Q4

Q4 owns deterministic replay/backtesting and paper execution downstream of canonical QNext Market Core data.

## Q4 acceptance scope

| Requirement | Q4 implementation |
| --- | --- |
| Deterministic replay | `BacktestEngine` with hashed dataset/run/result identity |
| Backtesting | next-bar execution, fees, slippage, PnL and drawdown |
| Paper engine | incremental `PaperEngine.process_bar()` using the same execution semantics |
| Chart markers | one canonical BUY/SELL marker emitted for every fill |
| Replay/paper parity | certification test feeds identical bars through replay and paper and requires identical fills, equity curve, markers and PnL |

## Ownership boundary

Q4 consumes canonical final QNext bars. It does not connect to Upstox or another provider, create canonical candles, revise market history, or sit in the provider realtime path.

```text
QNext Market Core
      │ canonical final bars
      ▼
Strategy Lab
├── deterministic replay / backtest
├── incremental paper engine
├── shared deterministic execution model
├── slippage + fee accounting
├── equity / drawdown attribution
├── chart execution markers
└── immutable file-backed replay artifacts
```

This remains compatible with the project constraints:

- no production Node.js server
- no application database
- file-backed durable replay output
- deterministic, versioned execution semantics

## Anti-look-ahead contract

A strategy receives history only through the current final bar. A signal timestamp must equal that bar close. Any generated order may fill only at that bar close boundary or later; under the reference model a bar-close signal fills at the next bar open.

The engine rejects non-final bars, duplicate or non-monotonic bars, mixed instrument/timeframe inputs, invalid OHLC envelopes, and bars whose quality is not `GOOD` or `RECOVERED`.

No future high, low, close, or revised candle value is made visible to the strategy.

## Shared replay/paper execution

Replay is intentionally implemented by driving the same incremental `PaperEngine` over a finite canonical bar set.

```text
final bar T
    ↓
strategy signal at T close
    ↓
pending paper order
    ↓
bar T+1 open
    ↓
shared qnext-exec-market-v1 fill
    ↓
fee + slippage
    ↓
position/equity update
    ↓
chart marker
```

This removes a separate "backtest fill path" that could silently drift from paper execution.

## Chart marker contract

Each fill emits a marker in replay and paper results:

```json
{
  "marker_id": "<session>-M000001",
  "instrument_id": "NSE:NIFTY50",
  "time_ms": 1790221560000,
  "action": "BUY",
  "price": 25104.25,
  "quantity_delta": 1.0,
  "label": "BUY 1 @ 25104.2500"
}
```

The browser/Vela layer can map these records directly to trade markers without recalculating strategy decisions.

## Reproducibility

Every replay derives:

- `dataset_hash` from canonical serialized input bars
- `strategy_definition_hash`
- `run_id` from dataset + strategy + execution configuration
- `result_hash` from fills, equity attribution and chart markers

Writing the same replay run again is idempotent. An existing run ID with different content fails closed.

## Certification

From `services/strategy-lab`:

```bash
PYTHONPATH=. python -m unittest discover -s tests -v
```

The certification includes an explicit `test_replay_paper_parity` gate. The package uses only the Python standard library.
