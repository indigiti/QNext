# QNext Python Intelligence — Q3/Q5/Q6

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
- controlled self-learning candidate generation
- chronological train/validation/untouched-test separation
- deterministic ridge/flat-threshold optimization on validation only
- candidate-vs-champion holdout comparison
- immutable DRAFT candidate registry
- explicit hash-locked promotion and rollback events

The prediction CLI does not fabricate future outcomes. Outcome evaluation is performed only when the complete declared future horizon is available.

## Runtime

Python 3.11+ with a standard-library runtime baseline.

Prediction example:

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

## Controlled self-learning workflow

The learning command consumes only completed immutable records from `features.jsonl`, `predictions.jsonl`, and `outcomes.jsonl`. It joins them by prediction/feature identity and rejects degraded data, incomplete joins, mixed feature schemas, and look-ahead candidate creation times.

```bash
PYTHONPATH=services/intelligence python -m qnext_intelligence.learning_cli train \
  --storage-root /private_html/qnext/storage/intelligence
```

`train` performs the complete research loop for the v1 learner:

`Analyze outcomes -> chronological split -> optimize on validation -> untouched holdout backtest -> compare with champion -> write DRAFT`

The learner uses 60% train, 20% validation and the final 20% as an untouched test window. Hyperparameters are selected using validation performance only. The current prediction direction stored with each historical prediction is the champion comparator on the same test rows.

Candidate records are stored in `models/candidates.jsonl`. A DRAFT never becomes production automatically. Promotion requires the exact candidate ID, model hash and an explicit approver:

```bash
PYTHONPATH=services/intelligence python -m qnext_intelligence.learning_cli promote \
  --storage-root /private_html/qnext/storage/intelligence \
  --candidate-id <candidate-id> \
  --expected-model-hash <sha256> \
  --approved-by <operator>
```

The production pointer is atomically written to `models/production.json`; immutable audit events are appended to `models/promotion_events.jsonl`. Rollback is explicit and uses a previously certified candidate:

```bash
PYTHONPATH=services/intelligence python -m qnext_intelligence.learning_cli rollback \
  --storage-root /private_html/qnext/storage/intelligence \
  --approved-by <operator>
```

Use `learning_cli status` to inspect DRAFT candidates and the production pointer.

## Promotion gates

The v1 candidate must satisfy all configured holdout gates before manual promotion is allowed:

- minimum untouched test sample count
- average strategy return no worse than the configured champion improvement threshold
- accuracy regression bounded by the configured tolerance
- maximum drawdown bounded by champion drawdown plus configured slack

These gates certify a DRAFT; they do not promote it.

## Architecture boundary

Go remains the canonical market/candle authority. Python failure must not interrupt REST history, WebSocket market delivery, or Vela rendering. Learning never edits live scripts or silently promotes a model. Production activation remains an explicit, auditable operation, and rollback remains available through the file-backed model registry.
