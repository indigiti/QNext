# Q5 Intelligence Exit Certification

Q5 establishes the deterministic intelligence and prediction contracts defined by Q0.

## Scope

Q5 provides feature snapshots, inference envelopes, immutable prediction records, degraded-data withholding, horizon-bounded outcome attribution, and file-backed append-only persistence. It does not make Python part of the Go realtime path and it does not enable live execution.

## Exit gates

- [x] Canonical finalized bars are the only feature input.
- [x] Bars closing after `as_of_time_ms` are excluded.
- [x] Forming bars are excluded.
- [x] Feature snapshots are deterministically hashed.
- [x] Model manifests bind feature-set, dataset and model hashes.
- [x] Training data must end strictly before inference time.
- [x] Predictions bind model and feature snapshot hashes.
- [x] Prediction identity is deterministic for the same decision context.
- [x] degraded/partial/stale/invalid inputs are withheld before model execution.
- [x] Outcomes reject withheld predictions.
- [x] Outcomes wait for the complete declared horizon.
- [x] MFE/MAE and terminal direction use only the declared horizon.
- [x] Prediction persistence is append-only and rejects identity mutation.
- [x] Q5 CI runs the intelligence certification tests.

## Explicitly deferred

- external ML training frameworks
- automated retraining
- model-selection automation
- live strategy/execution decisions
- external feature stores or databases
- online learning

Any later automated promotion path must preserve the Q0 rule that candidate models are validated before production and that production behavior never mutates silently.
