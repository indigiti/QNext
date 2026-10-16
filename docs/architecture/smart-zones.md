# RFC: QNeXT Smart Zones — universal adaptive architecture

Status: DESIGN ONLY / draft for staging-first implementation  
Target base: QNeXT main `ff36c71075df786f84dd0f07e0225d3f22d119c3` (PR #120)  
Decision: GO for isolated Zone Lab; not enabled on the trading chart by default.

## 1. Approved product preferences

| Property | Decision |
| --- | --- |
| Enabled by default | false |
| Eligible symbols | all, only when the symbol has valid finalized OHLC history |
| Eligible timeframes | all enabled/displayable QNeXT timeframes with enough history |
| Multi-timeframe confirmation | true; fail visibly and conservatively if unavailable |
| Learning mode | adaptive (rule-based; no fitting or automatic self-training) |
| Market data authority | unchanged Go Market Core and separate SYN+ research producer |
| Trading/alerts | prohibited in this phase; chart observations only |
| Environment | staging first |
| Rollback | immutable versioned detector/config profiles + atomic active pointer |

"All" means eligible on selection, NOT precompute every symbol/timeframe and NOT automatically request additional Upstox legs. Disabled-by-default takes precedence over all other settings.

## 2. Current interfaces and constraints

- `services/market-core/internal/httpapi/server.go` returns history using a bounded `limit`, along with `final`, `quality`, `authority_provider` and `revision`. `apps/workspace/src/qnext-provider.ts` can retrieve canonical bars; *never* confuse current forming bars with confirmed bars.
- `apps/workspace/src/qnext-indicator-engine.ts` currently accepts only `adaptive-ema-qalg` as a QNext-native kind; `services/ops-api/src/OpsController.php` validates `adaptive-ema-qalg` and `pine-v6`. Extend both registries and the public indicators catalog atomically when creating `smart-zones-v1`.
- Vela 0.7.7 `IndicatorModel.boxes?: DrawingBox[]` supports transient engine-produced rectangle models. Prefer these for auto zones, not persistent `chart.drawings.add('box')`. Manual user zones may use Vela's native drawing store, with separate ownership/IDs.
- Lightweight Charts currently renders candles, volume and paper markers; it needs a disposable pane/series primitive or bounded overlay for the very same immutable zones.
- `apps/workspace/src/charts/history-pagination.ts` caps browser history at 12,000 bars. Zones must support incremental historic prepend, deterministic rebuild and stable IDs, with no runaway cache.
- `docs/architecture/nifty-syn-plus.md` defines SYN+ as a shadow/research synthetic derived from research option-leg snapshots. Do not infer its past OHLC from spot NIFTY. The Go bars API may overlay *NIFTY spot volume* for SYN+; do not use this volume as genuine synthetic participation evidence.
- `docs/architecture/intelligence-lab.md` uses versioned numeric indicator features. Drawings alone do not satisfy its backtest feature contract: a numerical zone feature series is required.

## 3. Runtime architecture

`Go finalized bars/history -> per-view chart history adapter -> SmartZones Worker/Module -> immutable zone-state snapshots -> Vela auto-boxes | Lightweight primitive -> optional Zone Lab feature adapter`

- Pure deterministic detector in `apps/workspace/src/smart-zones/core/`, with no network/DOM/Go dependency. Inputs are sanitized finalized OHLC bars, instrument metadata, timeframe, verified HTF context and immutable profile.
- Host-side adapter in `apps/workspace/src/smart-zones/runtime/` fetches only requested symbol/timeframe history (plus one or two higher timeframes on demand), deduplicates bars by (symbol,timeframe,open time,revision), and serializes detector work in a Web Worker where supported. Release/cancel workers on chart removal/symbol switching; chart visibility drives computation, **not** canonical collection.
- One per-tab shared cache for the exact (symbol,timeframe,history revision,profile SHA) tuple and coalesced in-flight history reads. The same zone objects feed Vela and Lightweight renderers.
- Auto overlays never enter persistent user-drawing state. Manual drawings persist independently; if a user chooses "Save as manual" clone the zone with a distinct manual ID, with changes not fed back into detection labels.
- Active chart last-forming bar may be painted as a *preview only*, but must not confirm, score, invalidate, or train zones.

## 4. Detector v1: deterministic, no lookahead

The engine runs on completed candles in monotonic time order. Each state transition stores a finalized confirmation timestamp and input-data revision.

1. Filter out malformed candles, duplicates, quality-failed bars, nonpositive prices, missing timestamp, and non-final bars. Reject bad high-low-open-close ordering; skip or quarantine incomplete time spans rather than fabricating OHLC.
2. Compute Wilder ATR(14) from previously available finalized bars with at least 50 bars of warmup. Unitless normalized volatility and the instrument minimum tick, when verified, determine tolerance; **never** assume a fixed NIFTY point width applies to options or equities.
3. Swing pivot: initial 3 left and 3 right finalized bars. Pivot formed at candle `i` becomes **known** no earlier than the close of `i+3`. Equal highs/lows use a deterministic tie rule. Record `origin_bar_time` separately from `detected_at`.
4. Demand zone from a qualifying pivot-low candle: lower bound = wick low, upper bound = max(body open/close lower edge, lower bound + minimum tick/ATR thickness) with width capped and validated. Supply zone from a pivot-high candle: upper bound = wick high, lower bound = min(body upper edge, upper bound - minimum thickness). Clamp width to an ATR-relative cap; reject implausible unusually large wicks instead of inventing a new boundary.
5. Confirmation: from and after pivot detection, require a **future finalized** bar to show (a) displacement >= proposed `0.75*ATR`, and (b) structure break of an already known minor swing or an independently confirmed reversal. Confirmation must occur within a configurable bounded number of bars (initial 12). If unmet mark candidate expired; do not backdate activation. Mark `confirmed_at` at the finalized confirming candle's close.
6. HTF gate: where at least one verified higher timeframe exists, confirm against the **last completed HTF bar available as of `confirmed_at`**, with an optional second HTF for stronger alignment. Use the *same instrument* and session-aligned data. A missing/stale HTF is `CONTEXT_UNAVAILABLE`; do not silently treat it as an approving bullish/bearish signal.
7. A zone may become `ACTIVE`, `RETESTED`, `WEAKENED`, `INVALIDATED`, or `EXPIRED` only as finalized bars arrive. Proposed invalidation: two consecutive closes past the zone's outer boundary by >=0.1*ATR (computed as-of-bar). Track wicks separately to distinguish sweep from confirmed close breaks.
8. Merge/deduplicate zones of the same side when price boundaries overlap by >50% with similar anchor and confirmation times. Keep individual origin events in diagnostics; stable IDs derived from symbol, timeframe, side, origin timestamp and algorithm version; never from chart index.
9. Optional strength display 0-100 is a **heuristic**, not a win probability. Proposed feature weights: displacement 30, independent structure break 25, freshness 15, confirmed HTF alignment 20, reaction/retest quality 10. Renormalize only on *available verified* features and label unverified scores as provisional. Do not use SYN+ spot-volume proxy to boost strength.
10. Bound active overlay display to 6 buy and 6 sell zones (12 total) per chart; retain full audited event history only in separate Zone Lab. No drift of closed-zone history when more bars arrive.

Thresholds above are candidates for validation, **not** production-certified constants or learning results.

### State machine

`PIVOT_PENDING -> CANDIDATE -> CONFIRMED_ACTIVE -> RETESTED -> WEAKENED/INVALIDATED/EXPIRED`

- `PIVOT_PENDING` is not visible as an actionable signal.
- `CANDIDATE` is muted/research-only.
- `CONFIRMED_ACTIVE` requires the post-pivot finalized confirmation and the HTF rule, or is explicitly marked `LOCAL_ONLY_UNVERIFIED` if context is unavailable. Only confirmed zones may be labeled BUY ZONE / SELL ZONE.
- `INVALIDATED` is never removed retroactively from historical replay; it is hidden or faded based on user display settings. No trading execution follows a zone state.

## 5. Universal timeframe resolution

Timeframe **eligibility** is universal; context resolution is based on currently enabled/available session-aligned bars, not hardcoded to 15s.

| Primary | Preferred confirmed contexts |
| --- | --- |
| 5s / 15s / 30s | 1m and 5m; for 5s a 15s bridge is optional |
| 1m / 2m / 3m | 5m and 15m |
| 5m / 10m / 15m | 30m and 1h |
| 30m / 1h / 2h / 3h / 4h | next enabled higher intraday timeframe, then 1D |
| 1D | 1W / 1M |
| 1W | 1M |
| 1M | no higher confirmed timeframe available in v1 |

Resolution algorithm: prefer 2 larger supported timeframes, deduplicate, and never use a context shorter than the primary. Prefer session-aligned source bars over naive overnight or partial-period aggregation. No volume weighting by unrelated symbol. All HTF features are joined as-of close-time, never via future-completed HTF OHLC.

If zero usable higher timeframes exist, *retain ability to draw local-only candidate zones*, show `HTF unavailable` and withhold the "multi-timeframe confirmed" status. This preserves all-timeframe *coverage* without pretending unavailable HTF evidence exists. This exception is especially important on 1M.

## 6. Config/version/schema (file-backed, no MariaDB migration)

Private config snapshot path proposal: `config/smart-zones/profiles/<profile-id>/<version>.json`. Active pointer proposal `config/smart-zones/active.json`, atomically replaced with existing Ops AtomicFile helper; archived history + SHA256 checksum preserved. No direct unauthenticated writes. Enforce optimistic `expected_revision` and Admin token/session gate.

Example v1 profile:

```json
{
  "schema": "QNEXT.SMART_ZONES.PROFILE/1",
  "id": "adaptive-default",
  "revision": 1,
  "enabled_by_default": false,
  "all_symbols": true,
  "all_timeframes": true,
  "multi_timeframe_confirmation": true,
  "learning_mode": "adaptive",
  "confirmation_policy": "strict_when_context_available",
  "detector": {
    "left_bars": 3,
    "right_bars": 3,
    "atr_period": 14,
    "min_warmup_bars": 50,
    "min_displacement_atr": 0.75,
    "max_confirmation_bars": 12,
    "invalidation_closes": 2,
    "invalidation_buffer_atr": 0.1,
    "max_visible_zones_per_side": 6
  },
  "renderer": { "show_candidates": false, "show_invalidated": false },
  "execution": { "alerts": false, "orders": false, "automatic_promotion": false }
}
```

All patch endpoints must validate type/ranges, capture source author/time and immutable SHA, and retain at least the previously active snapshot. Rollback simply repoints to an older signed/hashed immutable snapshot, invalidates per-profile caches and redraws; it **never** changes historical candles. During initial staging, promotions require explicit Admin action; promotion to production remains a separate decision.

## 7. Zones event contract and feature bridge

Proposed immutable event fields:

```json
{
  "schema": "QNEXT.SMART_ZONES.EVENT/1",
  "zone_id": "stable deterministic id",
  "symbol": "QNEXT:NIFTY-SYN+",
  "timeframe": "15s",
  "side": "BUY",
  "lower_price": 22542.0,
  "upper_price": 22555.0,
  "origin_bar_time_ms": 0,
  "detected_at_ms": 0,
  "confirmed_at_ms": 0,
  "status": "CONFIRMED_ACTIVE",
  "context_timeframes": ["1m", "5m"],
  "context_status": "CONFIRMED",
  "strength_score": 0,
  "retest_count": 0,
  "source_revision": 0,
  "algorithm_version": "smart-zones-v1",
  "profile_sha256": "..."
}
```

The numbers above are **schema illustrations**, not observed zone levels or production recommendations. A zone must also carry invalidation and source-quality information in the full contract. Position/return outcomes are *not* embedded into event origins (would leak future information).

For the Lab, provide finalized-time-indexed numeric features such as zone side (+1/-1), normalized distance-to-nearest-zone, zone thickness/ATR, age-in-bars, strength, retest count, HTF verified flag, invalidation flag, and nearest opposite zone distance. Use the existing chart indicator contract or versioned extension. Lab data is embargoed from future outcomes until the training label horizon has elapsed. No auto orders and no auto ML fitting in "adaptive" mode.

## 8. Adaptive != self-learning

In selected `adaptive` mode, zone bounds, ATR thresholds and confidence context update **mechanically** as finalized data arrives under a fixed immutable profile. There is **no online parameter mutation**, trained classifier, background tuning service or automatic model promotion.

A later optional `lab-learning` mode can train proposed profiles off-thread on history and shadow data, but no profile is promoted without:
- chronological walk-forward split and untouched test segment;
- sufficient independent zones and multi-session/symbol/timeframe coverage;
- conservative treatment of intrabar double-touch;
- reproducible deterministic replay, and
- explicit Admin approval with rollback.

Different market families (indices, SYN/SYN+, cash equities, futures, options) may eventually have versioned calibration profiles, but shared fallback behavior must remain fully specified.

## 9. Acceptance tests and rollout

| Stage | Scope | Mandatory evidence |
| --- | --- | --- |
| Z0 | Freeze reference profiles + fixture bars + no-lookahead rules | replay fixtures, case matrix and checksum |
| Z1 | Pure detector + state machine | streaming-vs-batch equality, pivot delay, HTF as-of join, gap/quality invalidation, same-output cross-browser |
| Z2 | Vela indicator model boxes; manual drawing isolation | chart changes/pan/zoom/reconnect preserve zone identity, no persistent drawing pollution |
| Z3 | Lightweight pane primitive | same zone IDs/bounds/states as Vela, bounded draw and memory |
| Z4 | Private Admin config/version/rollback | auth checks, previous revision guard, atomic write and deterministic rollback |
| Z5 | Staging historical and market-hour replay | SYN+ 15s/30s/1m, NIFTY/SYN indices, an equity, a future, a liquid option, one daily/weekly symbol; >=20 trading sessions where history exists |
| Z6 | Optional Intelligence Lab numerical features | no future-feature leakage, detached from production model / execution |

Strict staging gates:
1. With zero opt-in, **zero zone calculations**, no extra upstream subscriptions, and existing chart/Go CPU baseline unchanged within measurement noise.
2. Confirm every zone is absent before its `confirmed_at_ms` in stepwise replay. Bar-by-bar and all-bars-at-once replay must have exactly matching event log and IDs when replaying the same finalized history.
3. A 1m HTF value formed at 09:16 cannot validate a 15s zone at 09:15:30. Session breaks, daylight/timezone and holiday gaps do not forward-fill fabricated HTF evidence.
4. Replay prefix isolation: the events up to time T must be identical whether the dataset stops at T or continues into the future. Labels of finalized history don't shift on panning or restoring the chart.
5. Incomplete SYN+ raw history, invalid snapshot quality, optional volume alias, and the absence of genuine synthetic volume cannot create synthetic confirmation bars or artificial high-confidence zones.
6. 100% agreement of emitted zone ID/bounds/status across chart renderers on shared fixtures; user manual edits must not mutate derived detector results.
7. CPU: benchmark detector and render separately on 1,500 and 12,000 bars under 1, 3 and 10 charts; propose acceptance targets only after obtaining staging baseline. The first version should run incremental updates with bounded zone display and caches.
8. Collect zone candidate, confirmed, retested and invalidated event counts; holdout evaluation compares false breakout and retest outcomes against a fixed no-zone baseline; **do not** claim predictive accuracy without such evidence.
9. Keep flags off for production until staging data-quality, replay and resource gates pass. Rollback must be one version-pointer update and restore previous renderer state without restarting Go services.

## 10. Initial engineering file plan

- `apps/workspace/src/smart-zones/core/{types,atr,detector,htf,events,score}.ts` — pure deterministic detector
- `apps/workspace/src/smart-zones/runtime/{worker,subscriptions,profile-client}.ts` — opt-in observer, shared local cache, lifecycle
- `apps/workspace/src/smart-zones/render/{vela-model,lightweight-primitive}.ts` — nonpersistent adapter per renderer
- `apps/workspace/test/smart-zones/*` — prefix equality, MTFA, quality/replay, UI lifecycle/renderer parity
- `apps/ops-console/src/smart-zones-settings.ts` — Admin profile management
- `services/ops-api/src/SmartZonesControl.php` — authenticated versioned config and rollback, gated by admin session/token
- `services/ops-api/tests/smart_zones.php` — persistence/auth/revision tests
- `config/smart-zones.example.json` — default disabled profile, included with core release, no ML wheelhouse dependency
- `docs/architecture/smart-zones.md` — this RFC
- Add an optional output snapshot adapter into Intelligence Lab after Z1–Z4 and historical validation; no changes to Market Core, collector or canonical persistence.

## 11. Decision and immediate next PR

Design approval does not change runtime settings. First implementation PR should contain only **Z0/Z1 pure detector + gold fixtures and regression tests**, with all charts unchanged. A subsequent separate Z2 PR wires opt-in native Vela boxes to the detector. Only after staging replay certification do we implement Lightweight renderer, Admin-config promotion or Lab features.

No execution or order placement is introduced in any of these stages.
