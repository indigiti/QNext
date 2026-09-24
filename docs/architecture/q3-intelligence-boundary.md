# Q3 Intelligence Boundary

Q3 activates the Python ownership boundary frozen in Q0 without placing Python in the realtime market-data path.

```text
Provider -> Go Market Core -> canonical finalized bars -> Python Intelligence
                         \-> Vela/browser
```

## Invariants

1. Go remains the sole realtime market-truth and candle authority.
2. Python consumes canonical bars; it does not rebuild provider candles.
3. Feature snapshots only use bars whose close time is less than or equal to the prediction time.
4. Only finalized bars are eligible for inference.
5. Feature sets, model versions, model hashes, prediction IDs, and decision context are deterministic and auditable.
6. Predictions are immutable records. Re-running the same decision context yields the same ID and duplicate persistence is rejected.
7. PARTIAL, STALE, DEGRADED, or INVALID data produces a `WITHHELD` prediction state without executing the model.
8. Outcomes are evaluated only after the complete future horizon is available.
9. Python failure never interrupts Go Market Core, chart history, or browser WebSocket delivery.
10. Q3 uses private file-backed durability and introduces no application database.

## Baseline model

Q3 deliberately ships a deterministic, versioned directional baseline rather than a self-mutating production ML system. The baseline establishes the complete feature -> prediction -> outcome lineage and certification surface. Future trained candidates can replace it only through explicit model manifests, validation, promotion, and rollback controls.
