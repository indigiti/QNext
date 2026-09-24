# Q2 — Symbol Registry and Provider Authority

Status: **IMPLEMENTED**

Q2 makes canonical instrument identity independent from any market-data provider and adds deterministic provider-authority selection.

## Canonical identity

QNext keeps one immutable canonical instrument ID for downstream consumers. Provider-specific keys are registered as mappings and are resolved before data enters the canonical pipeline.

Examples:

- canonical: `NSE:NIFTY50`
- Upstox mapping: `NSE_INDEX|Nifty 50`
- another provider may use a different key for the same canonical instrument

Charts, history, replay, strategies and intelligence code consume the canonical ID, not provider symbols.

## Authority policy

Authority is selected per canonical instrument from an ordered provider policy.

A provider is eligible only when all of the following are true:

1. the provider mapping is registered for the canonical instrument;
2. the provider is healthy;
3. the required market-data entitlement is present;
4. the latest observed event is within the configured staleness bound.

Among eligible providers, the lowest numeric priority wins. Equal priorities use provider name as a deterministic tie-break.

If no provider is eligible, QNext fails closed and returns no authority rather than silently accepting ambiguous or stale market truth.

## Q2 exit gates

- [x] provider-independent canonical instrument registry
- [x] provider-key to canonical-instrument resolution
- [x] multiple providers may map to the same canonical instrument
- [x] deterministic per-instrument authority priority
- [x] health gating
- [x] entitlement gating
- [x] freshness/staleness gating
- [x] deterministic failover
- [x] deterministic equal-priority tie-break
- [x] fail-closed behavior when no provider is eligible
- [x] unit certification for primary, failover, tie-break and no-authority states

The current production path still has Upstox as the only configured live provider, so authority selection is operationally trivial today. The Q2 contract is intentionally generic so adding other providers does not change downstream canonical contracts.
