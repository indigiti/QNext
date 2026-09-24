# ADR-0005: Deterministic provider authority

Status: Accepted

## Decision

Canonical QNext symbols select one authoritative provider server-side. Provider ticks are not arbitrarily averaged.

Authority selection considers health inputs such as connection state, last-tick age, latency, sequence continuity, gap rate, divergence, errors, and reconnect behavior.

Authority transitions must be deterministic and auditable.

## Required states

```text
PRIMARY_HEALTHY
PRIMARY_SUSPECT
FAILOVER_PENDING
SECONDARY_ACTIVE
FAILBACK_PENDING
```

Hysteresis/cooldowns must prevent provider flapping.
