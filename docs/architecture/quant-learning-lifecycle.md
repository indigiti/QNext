# QNext Quant Learning Lifecycle

This architecture extends QNext without changing the existing runtime ownership boundaries.

## Goal

Support the controlled lifecycle:

```text
Script
  -> Intelligence
  -> Analyze
  -> Optimize
  -> Backtest
  -> View Diff
  -> Self-Learning
  -> Apply Draft
  -> Promote
```

Historical observations and matured live observations may participate in training. A self-learning process may create a new candidate draft, but it may not silently replace a live strategy.

## Ownership

- **Go Market Core** remains the authority for realtime market truth, canonical bars, recovery and fan-out.
- **Python Intelligence** builds leakage-safe historical + live training datasets, features, models and candidate proposals.
- **Strategy Lab** owns deterministic backtest, paper/shadow evidence and strategy lifecycle promotion.
- **PHP / Ops** will expose configuration, permissions and controlled promotion actions in a later UI/control-plane slice.
- **Vela / TypeScript** will display analysis, strategy diff and lifecycle state in a later workspace slice.

## Hybrid training contract

`HybridTrainingSetBuilder` accepts observations tagged `HISTORICAL` or `LIVE`.

An observation is eligible only when:

1. its feature timestamp precedes its label timestamp;
2. its label/outcome is known by the dataset `as_of_ms`;
3. its quality is `GOOD` or `RECOVERED`; and
4. its label does not cross the train/validation/test boundary.

Boundary-crossing observations are purged. Dataset hashes are deterministic and include the split boundaries, preventing accidental reuse under different temporal partitions.

This establishes the baseline for rolling and walk-forward retraining without look-ahead leakage.

## Candidate and promotion contract

A learned or optimized change is materialized as a `StrategyRevision` in `DRAFT`.

The allowed path is:

```text
DRAFT -> BACKTESTED -> PAPER -> SHADOW -> APPROVED -> LIVE -> RETIRED
```

Required evidence is fail-closed:

- `BACKTESTED` requires passed `BACKTEST` evidence.
- entering `SHADOW` requires passed `PAPER` evidence.
- entering `APPROVED` requires passed `SHADOW` evidence.
- entering `LIVE` requires passed `HUMAN_APPROVAL` evidence.

Automated promotion is blocked when a candidate changes protected risk/permission paths, and automated promotion may not enter `APPROVED` or `LIVE`.

Every lifecycle state can be persisted as an immutable file-backed snapshot, so this remains compatible with QNext's no-database validation phase and later MariaDB migration.

## Protected settings

The first protected set includes:

- maximum daily loss;
- maximum position;
- capital ceiling;
- kill-switch settings;
- user/strategy permissions; and
- broker permissions.

Self-learning may propose such changes for review, but it may not auto-promote them.

## Next slices

The foundation in this change intentionally does **not** send live broker orders. Follow-up integration should add:

1. model/parameter optimizer adapters and walk-forward orchestration;
2. strategy diff metrics and equity/trade comparison;
3. paper + shadow evidence emitters;
4. Ops API endpoints for draft review and promotion;
5. Quant workspace UI; and
6. Go risk/OMS authority before any broker-side live execution.
