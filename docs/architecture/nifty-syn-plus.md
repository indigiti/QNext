# NIFTY-SYN+ shadow synthetic

## Status

`QNEXT:NIFTY-SYN` remains the stable production synthetic and is not modified by this feature.

`QNEXT:NIFTY-SYN+` is a parallel research/shadow synthetic derived from the Data Collector v1 Upstox research connection. It is intentionally not promoted over the production symbol.

## Price construction

SYN+ keeps the same put-call-parity structure used by the production synthetic:

`strike + call fair price - put fair price`

For each current-expiry call/put pair, fair price is selected in this order:

1. top-of-book microprice when bid, ask and quantities are valid;
2. bid/ask midpoint when a valid two-sided quote exists but quantity is unavailable;
3. LTP as the final fallback.

The synthetic value is the median of accepted strike-pair values. Current production safety limits are reused by default: minimum valid candidates, maximum leg age and maximum call/put timestamp skew.

Only the current expiry contributes to SYN+. The next expiry remains captured by Data Collector v1 for research, IV surface and term-structure work, but is not mixed into the synthetic index value.

## Isolation

SYN+ runs as a second research sink behind `qnext-data-collector`. The production Market Core synthetic assembler, its symbol and its WebSocket output remain untouched.

The same research envelope is fanned out to:

- option-state persistence (1-second and 15-second snapshots);
- SYN+ shadow calculation (1-second snapshots).

This keeps richer research processing off the latency-sensitive production feed path.

## Output

Default instrument: `QNEXT:NIFTY-SYN+`

Default version: `nifty-syn-plus-v1`

Snapshots are stored at:

`storage/research/synthetic/1s/YYYY/MM/YYYY-MM-DD.jsonl`

Schema: `QNEXT.RESEARCH.SYNTHETIC_SNAPSHOT/1`

Each snapshot records value, live underlying spot, basis to spot, current expiry, ATM, candidate counts, price-source counts, median absolute deviation and per-strike acceptance/rejection diagnostics.

## Runtime overrides

- `QNEXT_SYN_PLUS_INSTRUMENT_ID`
- `QNEXT_SYN_PLUS_VERSION`

The selected market still comes from `QNEXT_DATA_COLLECTOR_MARKET` (NIFTY by default).

## Promotion gate

SYN+ is a shadow/research instrument in v1. Promotion to chart/trading authority requires live comparison against `NIFTY-SYN`, stability/latency checks and an explicit later release decision. No automatic promotion or replacement is performed by this change.
