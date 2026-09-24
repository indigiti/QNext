# QNext Python Intelligence — Q3

This service consumes only canonical, finalized QNext Market Core bars. It is not part of the realtime chart path and must never become a dependency of Go Market Core availability.

## Q3 responsibilities

- leakage-safe feature generation from finalized bars only
- deterministic feature snapshots with lineage hashes
- regime classification
- versioned baseline prediction model
- immutable prediction IDs and decision-context hashes
- horizon-based MFE/MAE/return outcome attribution
- append-only JSONL persistence with duplicate-key rejection

## Runtime

Python 3.11+; no database and no runtime third-party dependencies are required for the Q3 baseline.

Example:

```bash
python -m qnext_intelligence.cli \
  --market-core-url http://127.0.0.1:8080 \
  --instrument-id QNEXT:NIFTY \
  --timeframe 1m \
  --from-ms 1790307900000 \
  --to-ms 1790312400000 \
  --storage-root /private_html/qnext/storage/intelligence
```

The service writes `features.jsonl`, `predictions.jsonl`, and `outcomes.jsonl` below the configured private storage root.
