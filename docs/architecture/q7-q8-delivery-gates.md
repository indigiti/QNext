# Q7 and Q8 delivery gates

Q7 is the mandatory foundation before Q8 increases market-data demand. This is a delivery backlog, not an exit certification.

## Q7 Universal Foundation & Reliability

- Q7.1: session-aware, independently scoped feed readiness. Preserve process liveness, persistence checks and existing provider status; requested but failed discovery is unready, disabled feeds do not block readiness. Continuous calendar behavior is metadata-driven rather than matched to OKX by name. This patch implements this slice; CI and staging checks remain required.
- Q7.2: provider capability and lifecycle registry, canonical instrument registration from validated config, collision detection, provider-key mappings, calendar references and explicit activation. Remove provider-specific branching from candle bucketing and shared storage/history only after compatibility tests for SYN/SYN+, 5s and 1m.
- Q7.3: certify another exchange via an adapter and configuration only; no duplicate candle, history, stream, or indicator engine. Validate disabled/failed provider isolation, reconnect, backfill, revisions and calendar holidays.
- Q7.4: reproducible dependency locks and verified build inputs; use locked installs, measure artifact reproducibility, keep runtime secrets and history outside releases.
- Q7.5: selective release transport. Reuse existing inventory hashes, bind exact base/target and deletions, verify reconstructed full runtime before atomic activation, retain rollback and full ZIP fallback. Do not enable partial ZIP overlays in DigiOps before installer support is certified.
- Q7.6: live certification with bounded monitoring and provider-specific freshness/persistence/gap/volume evidence. Require staging observations and documented performance budgets.

Exit gate: a newly certified exchange can be added without duplicating the market engine; independent feed failures cannot masquerade as healthy ingestion. CI green alone is not live certification.

## Q8 Professional Trading Workspace

Start after Q7 certification; feature flags and independent modules prevent optional features from adding unbounded subscriptions or per-symbol workers.

- Q8.1: reusable watchlist/preset storage, validated canonical instrument IDs, ownership and bounded list sizes; integrate with existing symbol selection.
- Q8.2: synchronized charts with explicit linking groups, loop prevention and preserved per-chart indicator settings. Reuse Vela workspace and existing shared transport.
- Q8.3: server-side shared calculation service and bounded scanner batches; cache by canonical symbol, timeframe, parameters and data revision; expose freshness and avoid lookahead.
- Q8.4: durable server-side alert rules, evaluation on confirmed candle/revision semantics, deduplication, bounded queue, retry and delivery receipts. Existing browser chart alert toggle is not durable server-side alerting.
- Q8.5: market overview reuses scanner snapshots and watchlist calculations rather than fetching per tile; enforce concurrency, memory, subscription and latency budgets.

Exit gate: identical calculations are shared across users/features where appropriate, alert delivery is verified across restarts/retries, and resource budgets pass representative load and failure tests.

No Q8 feature is certified by this Q7.1 change.
