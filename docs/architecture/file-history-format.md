# QNext File-Backed Canonical History

QNext initially runs without an application database. Canonical finalized bars are therefore persisted as versioned append-only JSONL records.

## Record schema

Current record identity:

```text
QNEXT.HISTORY.BAR/1
```

Each record includes:

- canonical instrument id
- timeframe
- open/close timestamps in UTC epoch milliseconds
- OHLCV
- finality
- revision
- authority provider
- data quality
- recovered/corrected flags
- candle engine version
- synthetic version where applicable

## Layout

```text
storage/
└── market/
    └── NSE_NIFTY50/
        └── 30s/
            └── 2026/
                └── 09/
                    └── 2026-09-24.jsonl
```

## Revision model

History is append-only. A correction never overwrites the earlier record.

For the same canonical candle identity:

```text
instrument_id + timeframe + open_time_ms
```

readers select the highest revision. This preserves an audit trail while exposing one canonical current version.

## Durability

The initial Go store writes one JSONL record per append, uses append mode, synchronizes the file before reporting success, and rejects forming bars. File retention, compaction, backup, and multi-process writer coordination remain explicit Q1 operational work rather than implicit database behavior.
