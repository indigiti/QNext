# Q3 — Python Intelligence Exit Certification

Status: **COMPLETE**

Q3 is complete when the Q3 workflow is green on the completion PR and the service remains downstream-only from Go Market Core.

## Canonical intelligence path

- [x] canonical Market Core history client
- [x] finalized-bar-only feature input gate
- [x] strictly increasing event-time validation
- [x] deterministic feature-set version
- [x] deterministic feature snapshot hash
- [x] regime classification
- [x] versioned deterministic baseline model
- [x] immutable prediction ID
- [x] model hash and decision-context hash
- [x] data-quality propagation
- [x] degraded-data prediction withholding
- [x] horizon-based return/MFE/MAE outcome attribution
- [x] append-only file-backed feature/prediction/outcome persistence
- [x] duplicate immutable-record rejection

## Architecture gates

- [x] no Python dependency in Go realtime/chart path
- [x] no production Node.js server introduced
- [x] no application database introduced
- [x] no provider credentials handled by Python
- [x] no future bar may enter a prediction feature window
- [x] prediction model is versioned and does not self-mutate

## Certification tests

- [x] deterministic feature hash
- [x] leakage/finalized-bar gate
- [x] deterministic prediction identity
- [x] probability normalization
- [x] degraded-quality withholding
- [x] horizon completion outcome gate
- [x] immutable JSONL duplicate rejection
- [x] final CI PASS

## Deployment note

Production should point the service at the private QNext Market Core URL and write under `private_html/qnext/storage/intelligence/`. Model training/retraining is intentionally outside this baseline runtime; candidate promotion will be a later controlled lifecycle step.
