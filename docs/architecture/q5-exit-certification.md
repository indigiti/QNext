# Q5 — Python Intelligence Exit Certification

Status: **CANDIDATE**

Q5 establishes the first controlled intelligence lifecycle while preserving the frozen ownership boundary: Go remains the realtime market authority and Python consumes canonical data asynchronously.

## Point-in-time features

- [x] versioned feature-set identity
- [x] per-feature source timestamps
- [x] hard rejection of future information
- [x] deterministic snapshot hashing
- [x] append-only feature snapshot storage

## Controlled model lifecycle

- [x] immutable model manifests
- [x] append-only lifecycle events
- [x] candidate → validated → production gate
- [x] validation evidence bound to the training dataset hash
- [x] explicit production approval identity
- [x] previous production version retired through an auditable event

## Predictions and outcomes

- [x] production-model-only prediction creation
- [x] model/version/hash and feature-snapshot lineage
- [x] deterministic decision-context hash
- [x] immutable prediction and outcome records
- [x] probability validation
- [x] prediction-horizon timing gate
- [x] return, actual-direction and correctness attribution

## Runtime constraints

- [x] no production Node.js server introduced
- [x] no application database introduced
- [x] Python is not inserted into the critical chart-data path
- [x] no provider or broker credential is required for certification
- [x] standard-library-only certification tests

## Exit gate

Run `make q5-check`. Q5 is complete when this workflow passes in CI. Live model training, external ML libraries, scheduled retraining, and market-specific feature packs remain follow-on work and must preserve the same point-in-time and promotion contracts.
