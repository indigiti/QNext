# QNext Strategy Lab — Q4

Q4 adds the deterministic strategy/replay plane without changing QNext Market Core authority.

## Ownership boundary

Q4 consumes canonical final QNext bars. It does not connect to Upstox or another provider, create canonical candles, revise market history, or sit in the browser realtime path.

```text
QNext Market Core
      │ canonical final bars
      ▼
Strategy Lab
├── deterministic replay
├── strategy signal evaluation
├── next-bar execution model
├── slippage + fee accounting
├── equity / drawdown attribution
└── immutable file-backed run artifacts
```

This remains compatible with the project constraints:

- no production Node.js server
- no application database
- file-backed durable run output
- deterministic, versioned execution semantics

## Anti-look-ahead contract

A strategy receives history only through the current final bar. A signal timestamp must equal that bar close. Any generated order may fill only on a later bar; the reference model fills at the next bar open.

The engine rejects non-final bars, duplicate or non-monotonic bars, mixed instrument/timeframe inputs, invalid OHLC envelopes, and bars whose quality is not GOOD or RECOVERED.

No future high, low, close, or revised candle value is made visible to the strategy.

## Reference execution

```text
signal on final bar T close
        ↓
pending market order
        ↓
fill at bar T+1 open
        ↓
deterministic configured slippage
        ↓
deterministic configured fee
```

End-of-run liquidation is explicit and separately labelled.

## Reproducibility

Every run derives dataset_hash from canonical serialized input bars, strategy_definition_hash, run_id from dataset + strategy + execution configuration, and result_hash from fills and equity attribution.

Writing the same run again is idempotent. An existing run ID with different content fails closed.

## Local certification

From services/strategy-lab:

```bash
PYTHONPATH=. python -m unittest discover -s tests -v
```

The package uses only the Python standard library.
