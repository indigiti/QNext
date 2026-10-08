# Q7 Production Hardening

Status: In progress

Q7 hardens the certified Q1-Q6 runtime without changing canonical market ownership,
strategy semantics, chart behavior, or broker-execution boundaries.

## Q7.1 production readiness

`/health` remains a process-liveness signal.

`/ready` is the production-role gate. When `QNEXT_REQUIRE_LIVE_READY=1` it fails
closed if live market configuration is absent. During an active configured market
session it also requires fresh underlying feed observations.

Readiness additionally observes canonical history persistence:

- pending write errors;
- oldest flush lag;
- async writer queue utilization.

Temporary provider startup is handled by deployment smoke retries. The supervisor
continues using `/health` for process replacement so a transient provider outage
does not create a restart loop.

## Q7.2 interactive history guardrails

The public chart `/api/v1/bars` endpoint is intentionally not a bulk research
export API.

It now enforces:

- supported timeframe validation;
- timeframe-specific maximum request windows;
- a hard maximum response size of 50,000 bars;
- an optional caller `limit` between 1 and 50,000.

Oversized requests fail before filesystem history scanning whenever the requested
window itself exceeds the certified interactive bound.

## Q7.3 persistence isolation

History file synchronization is partition-scoped instead of store-global.

A scan of an old instrument/timeframe/day file therefore cannot block canonical
persistence to an unrelated live partition. Readers and writers of the same JSONL
partition still share an RW lock so readers cannot observe a partially-written
record.

Finalized bars are still never silently dropped. Disk failures remain visible
through readiness and persistence telemetry.

## Q7.4 release-path unification

Both Cloudways/user-mode supervision and systemd/root-mode deployment use the
same versioned Market Core contract:

```text
qnext-market-core.current
        ↓
qnext-market-core-<commit>
```

The systemd service starts through `qnext-market-core-launcher`, which validates
the pointer before executing the versioned binary.

## Q7.5 contract drift prevention

The checked-in OpenAPI route inventory is aligned to the implemented Go HTTP
surface. Q0 now compares the OpenAPI path set to the Market Core route
registrations and fails on missing or stale conventional HTTP routes.

The realtime `/api/v1/stream` contract remains governed by AsyncAPI.

## No behavioral changes to

- provider authority;
- candle calculation;
- synthetic calculation;
- WebSocket replay/fan-out;
- Strategy Lab fills;
- Intelligence promotion;
- broker execution.

## Next Q7 slices

After this foundation is deployed and observed:

1. dependency lockfiles and supply-chain/SBOM hardening;
2. Admin login throttling and session-generation revocation;
3. collision-free canonical history path encoding;
4. partition/index strategy for growing Intelligence JSONL stores;
5. server-side always-on certified indicator feature execution;
6. file-backed/MariaDB shadow-readiness evidence after the live validation gate.
