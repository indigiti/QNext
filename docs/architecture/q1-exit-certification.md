# Q1 — Market Truth Exit Certification

Status: **CANDIDATE**

Q1 is complete when the final completion PR passes the Q0 contract workflow and the Q1 Go workflow including race tests, build, and all certification tests.

## Canonical market path

- [x] Upstox V3 authorization
- [x] concrete WebSocket transport
- [x] V3 Protobuf LTPC decoding
- [x] provider-independent symbol registry
- [x] deterministic normalization
- [x] bounded duplicate suppression for repeated LTPC snapshots
- [x] canonical candle engine
- [x] append-only file history with revisions
- [x] 1m/3m/5m Upstox V3 intraday recovery
- [x] fail-closed rule for unrecoverable 15s/30s disconnect gaps
- [x] reconnect supervisor with bounded exponential backoff

## NIFTY-SYN

- [x] exactly five configured strikes / ten option legs
- [x] call/put pair validation
- [x] stale-leg and leg-time-skew rejection
- [x] median candidate aggregation
- [x] synthetic version lineage carried into canonical candles
- [x] synthetic ticks use the same candle/history pipeline as NIFTY

## Browser delivery

- [x] one /api/v1/stream WebSocket
- [x] multiple simultaneous bar subscriptions
- [x] stable stream IDs
- [x] monotonic per-stream sequence
- [x] bounded replay buffer
- [x] RESUME
- [x] RESYNC_REQUIRED when replay is unavailable
- [x] slow-subscriber bounded buffer/drop behavior

## Certification

- [x] deterministic NIFTY fixture
- [x] deterministic NIFTY-SYN fixture
- [x] candle finality tests
- [x] synthetic parity tests
- [x] duplicate suppression tests
- [x] history/live boundary test
- [x] stream resume/replay tests
- [x] provider reconnect/recovery tests
- [ ] final CI PASS

## Deployment note

The code path is certifiable without live credentials. A production environment still needs:

1. UPSTOX_ACCESS_TOKEN supplied server-side;
2. QNEXT_MARKET_CONFIG pointing to a private configuration based on config/q1-market.example.json;
3. current option instrument keys for the selected five-strike/expiry set.

A live-credential smoke test is an environment/deployment validation, not a reason to expose credentials in repository CI.
