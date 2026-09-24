# Q3 — Python Intelligence Exit Certification

Status: **IMPLEMENTED / CI CANDIDATE**

Q3 activates the Python intelligence ownership boundary without placing Python in the realtime market-data path.

## Canonical intelligence path

- [x] Market Core `/api/v1/bars` history client
- [x] finalized-bar-only feature input gate
- [x] strictly increasing history validation
- [x] deterministic feature-set version
- [x] deterministic feature snapshot hash
- [x] deterministic regime classification
- [x] versioned deterministic baseline model
- [x] immutable prediction ID
- [x] model hash and decision-context hash
- [x] data-quality propagation
- [x] degraded-data prediction withholding
- [x] horizon-based return/MFE/MAE outcome attribution library
- [x] append-only file-backed feature/prediction/outcome persistence primitive
- [x] duplicate immutable-record rejection
- [x] runnable history -> feature -> prediction CLI

## Architecture gates

- [x] no Python dependency in Go realtime/chart path
- [x] no production Node.js server introduced
- [x] no application database introduced
- [x] no provider credentials handled by Python
- [x] no future/forming bar may enter a prediction feature window
- [x] prediction model is versioned and does not self-mutate
- [x] outcomes are not generated until the complete declared horizon exists

## Certification

The Q3 workflow compiles the package, runs the full intelligence unit/certification suite, and validates the CLI surface on Python 3.11.

Production should point the service at the private QNext Market Core URL and write under `private_html/qnext/storage/intelligence/`. Automatic training/retraining and candidate promotion remain separate controlled lifecycle work.
