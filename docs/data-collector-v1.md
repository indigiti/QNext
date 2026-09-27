# QNext Data Collector v1

Data Collector v1 records the live option-market state that cannot be reconstructed later from ordinary historical candles. It runs beside `qnext-market-core` and uses a separate Upstox Market Data Feed V3 connection so research capture cannot stall the chart/trading fan-out.

## Subscription plan

For the selected underlying, the collector subscribes to spot in `ltpc` mode, resolves the current and next option expiry from the Upstox option-contract catalog, and rebuilds the option basket whenever the current-expiry ATM strike changes.

Default NIFTY research basket:

- current expiry ATM ±20 strikes, CE + PE;
- next expiry ATM ±20 strikes, CE + PE;
- current expiry ATM ±5 strikes use `full` mode for 5-level depth and market state;
- all other selected contracts use `option_greeks` mode for first-level bid/ask, Greeks, IV, OI and volume.

With a complete 41-strike window this is 164 option contracts: 22 in `full` and 142 in `option_greeks`. The primary Market Core connection remains unchanged.

## Persisted research records

The collector maintains the latest state per selected option and emits deterministic JSONL snapshots at 1-second and 15-second boundaries under:

```text
storage/
  research/options/1s/YYYY/MM/YYYY-MM-DD.jsonl
  research/options/15s/YYYY/MM/YYYY-MM-DD.jsonl
```

Schema: `QNEXT.RESEARCH.OPTION_SNAPSHOT/1`.

Each record includes provider/instrument metadata, underlying price, expiry/strike/side, LTP/LTT/LTQ, five-level depth where available, top bid/ask quantities, spread, mid, microprice, depth imbalance, volume, OI, IV, delta/gamma/theta/vega/rho, ATP and total bid/ask quantity.

Optional raw protobuf-frame retention can also be enabled. Raw capture remains short-retention operational data; 1s/15s research snapshots are the durable research contract.

## Runtime

Build and run:

```bash
go build ./cmd/qnext-data-collector
UPSTOX_ACCESS_TOKEN=... ./qnext-data-collector
```

Environment variables:

- `UPSTOX_ACCESS_TOKEN` — required.
- `QNEXT_STORAGE_ROOT` — defaults to `./storage`.
- `QNEXT_MARKET_CONFIG` — optional existing QNext market config; defaults to built-in markets.
- `QNEXT_DATA_COLLECTOR_MARKET` — defaults to `NIFTY`.
- `QNEXT_DATA_COLLECTOR_WING_STRIKES` — defaults to `20`.
- `QNEXT_DATA_COLLECTOR_FULL_WING_STRIKES` — defaults to `5`.
- `QNEXT_DATA_COLLECTOR_RAW_CAPTURE=1` — retain raw research WebSocket frames under `raw/upstox-research/...`.

## Storage migration boundary

v1 deliberately uses the existing file-backed QNext architecture. Feed decoding, basket planning and snapshot production do not depend on JSONL semantics beyond the final writer, so a later MariaDB/columnar research sink can implement the same snapshot contract without changing Upstox ingestion.

## Not in v1

Data Collector v1 records the inputs for volatility and microstructure research. IV surfaces, skew/term-structure features, realized-volatility forecasts, probability/alpha models and execution decisions remain downstream consumers and should not be coupled into ingestion.
