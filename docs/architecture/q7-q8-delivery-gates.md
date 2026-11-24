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

## Q7.2/Q7.3 direct-feed foundation

The optional private JSON file selected by `QNEXT_DIRECT_FEEDS_CONFIG` registers
additional, explicitly enabled direct feeds via a startup adapter registry.
See `config/qnext-direct-feeds.example.json`. Existing OKX gold admin toggles
are unchanged and are not accepted as duplicate manifest entries. Additional
symbols are limited to eight. Invalid configuration fails closed for additional
feeds and is visible in readiness; existing feeds are not disabled.

The initial production adapter supports exact, live OKX linear swaps only,
verifying base/settlement currencies and finite positive contract size before
atomic symbol registration. Existing X-Perp uses its separate discovery rule.
All compatible direct feeds share one canonical candle pipeline and clock;
history, broker, and normalized tick contracts are reused. Startup REST
backfills are limited to two in flight. Each symbol still has its own provider
subscription and reconnect state; exchange multiplexing remains a later task.

This is not universal exchange certification. Upstox/Dhan synthetic/volume
routing remains unchanged. The candle/rollup session policy still contains
exchange-specific branches, so a non-OKX adapter must not be activated until
that policy is generalized and its volume/timeframe/recovery contract is
certified. No external exchange or new symbol is enabled by this patch alone.

## Q7 remaining foundation work

- Direct-feed continuous UTC policy now comes from the verified registry lane and
  immutable instrument identity set; the same policy is used in the candle engine,
  finalization clock and both rollup stages. Legacy NSE/BSE and SYN/SYN+ behavior
  remains unchanged. Other calendar types still require their own certification.
- Q7 Foundation CI generated npm lockfiles using the package registry. Both
  candidates were verified against their CI SHA-256 digests and committed. The
  four npm build workflows now use npm ci; the exact head still requires CI.
- `scripts/release/release_delta.py` implements exact-base changed-file transport
  and deletion-aware reconstruction into a new directory. The approved base SHA,
  target SHA and target inventory digest must come from a separate trusted channel.
  Full payload inventory is verified before reconstruction is exposed. Existing
  DigiOps full release remains the production-compatible artifact. Do not apply a
  delta over the running release or claim service activation/rollback certification.
- A full trading session with actual live sources, gap/revision evidence and
  workload-aware performance evidence remains a mandatory external exit gate.
  No CI fixture is a substitute for live certification.

The optional live integrity monitor includes active direct UTC instruments, with
bounded per-series publication/final-event evidence and gap checks across day
boundaries. Missing intervals are counted in constant time rather than rescanned
on every finalization clock cycle. These are publication observations, not
broker trade-loss certification. Raw capture remains provider-specific.
