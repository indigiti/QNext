# QNext Python Intelligence — Q3/Q5

QNext Intelligence consumes canonical, finalized bars from the Go Market Core. It is downstream-only: Python never owns provider connectivity, candle construction, browser fan-out, or chart availability.

## Implemented baseline

- Market Core `/api/v1/bars` history client
- strict increasing-time validation
- finalized-bar-only feature windows
- anti-look-ahead filtering by `as_of_time_ms`
- deterministic feature snapshots and hashes
- deterministic baseline regime/direction model
- immutable prediction IDs and decision-context hashes
- explicit degraded-data withholding
- horizon-bounded outcome attribution library
- append-only JSONL feature/prediction/outcome stores
- duplicate immutable-record rejection
- CLI for history -> feature -> prediction persistence

The CLI does not fabricate future outcomes. Outcome evaluation is performed only when the complete declared future horizon is available.

## Runtime

Python 3.11+ with a standard-library runtime baseline.

Example:

```bash
PYTHONPATH=services/intelligence python -m qnext_intelligence.cli \
  --market-core-url http://127.0.0.1:8080 \
  --instrument-id QNEXT:NIFTY \
  --timeframe 1m \
  --from-ms 1790307900000 \
  --to-ms 1790312400000 \
  --storage-root /private_html/qnext/storage/intelligence \
  --lookback 20 \
  --horizon-bars 3
```

The command writes immutable `features.jsonl` and `predictions.jsonl` records below the configured private storage root.

## Architecture boundary

Go remains the canonical market/candle authority. Python failure must not interrupt REST history, WebSocket market delivery, or Vela rendering. Model training and automatic promotion remain deferred behind explicit model manifests and certification.
