# QNext Python Intelligence

Q5 establishes a database-free Python intelligence foundation that consumes canonical market data asynchronously. It is deliberately outside the Go realtime chart path.

## Guarantees

- Feature snapshots are point-in-time and reject any feature whose source timestamp is later than the snapshot `as_of_time_ms`.
- Snapshot identities are deterministic SHA-256 hashes over canonical content.
- Model manifests are immutable and versioned.
- Model lifecycle transitions are append-only events.
- Candidate models cannot become production until validation evidence explicitly passes.
- Predictions may only use an explicitly promoted production model.
- Predictions are immutable, version-bound, feature-snapshot-bound, and decision-context hashed.
- Outcomes cannot be evaluated before the declared prediction horizon has closed.
- Storage is structured JSONL under the configured private runtime root; no application database is required.

## Logical runtime layout

```text
storage/
├── features/snapshots.jsonl
├── models/manifests.jsonl
├── models/lifecycle.jsonl
├── predictions/predictions.jsonl
└── outcomes/outcomes.jsonl
```

The caller supplies the root directory, so production should point it at QNext's private storage tree.

## Certification

```bash
make q5-check
```

The Q5 suite is standard-library-only and requires no network access or model-provider credentials.
