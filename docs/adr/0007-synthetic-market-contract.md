# ADR-0007: Synthetic market construction is versioned and quality-gated

Status: Accepted

## Context

The validated QNext NIFTY synthetic method uses option parity candidates around ATM:

```text
candidate = strike + call_price - put_price
synthetic = median(valid candidates)
```

Baseline strike set:

```text
ATM - 100
ATM - 50
ATM
ATM + 50
ATM + 100
```

## Decision

Every synthetic instrument uses a versioned `SyntheticDefinition` that freezes:

- underlying
- expiry-selection policy
- ATM-selection policy
- strike interval and offsets
- leg price field
- maximum leg age
- maximum call/put timestamp skew
- minimum valid candidate count
- aggregation rule

Every calculation retains candidate-level lineage.

Stale, missing, or temporally incompatible option legs are rejected explicitly. If too few valid candidates remain, QNext reports degraded or invalid quality rather than silently publishing a confident synthetic value.
