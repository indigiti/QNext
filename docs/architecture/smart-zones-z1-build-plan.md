# QNeXT Smart Zones — Z1 executable design & acceptance contract

Status: DESIGN ONLY, no runtime hooks / public endpoints / deployed feature.
Companion: `docs/architecture/smart-zones.md` and `config/smart-zones.example.json`.

## 1. Implementation order and ownership

The first development PR is **Z1**: build a pure TypeScript detector and exhaustive
fixtures under `apps/workspace/src/smart-zones/core/` and
`apps/workspace/test/smart-zones/`. Do not wire Z1 to either chart,
Market Core, data collector, intelligence training, or public API.

Suggested files:

- `types.ts`: finalized OHLCV, time/session metadata, frozen detector profile,
  higher-timeframe evidence, immutable zone/event records.
- `atr.ts`: Wilder true range and ATR; no future-bar use.
- `structure.ts`: confirmed swing/pivot discovery using left/right windows.
- `htf.ts`: completed higher-timeframe as-of-close join and unavailable semantics.
- `detector.ts`: state transitions, distance/geometry, merge, invalidation.
- `score.ts`: explainable 0–100 heuristic only; not prediction probability.
- `replay.ts`: streaming/batch fixture harness with prefix snapshots.
- `stable-id.ts`: deterministic IDs independent of array indices.

Only after Z1's tests are green should Z2 add transient Vela
`IndicatorModel.boxes` in a separate PR. The automatic chart overlay stays
**disabled by default** through Z1–Z5.

## 2. Contracts: time, candles, revisions, market family

`FinalizedBar` must include: symbol ID, timeframe, open timestamp in epoch-ms,
close timestamp in epoch-ms, OHLC, final flag, source-quality, revision and
optional provider metadata. The source returns *open* timestamps. The adapter
derives/validates close timestamps using the exchange calendar/session
schedule; it must not simply add nominal seconds across weekends/holidays
or over a trading-session boundary.

A bar with `final !== true`, incomplete timestamp, nonfinite OHLC,
`high < max(open,close)`, `low > min(open,close)`, `high < low`, or
unaccepted quality is quarantined. Never fill gaps using a future candle.

If a correction arrives with higher revision for the same bar, recompute
from the last safe checkpoint and emit a **revision event**. Zones that
change historically because of corrected source data must be clearly
identified as a corrected replay, not silently repainted by the detector.

Timeframe support is capability-based. The feature is eligible for any
enabled chart timeframe and instrument with the required finalized bar
history, but no background scan of every symbol or extra upstream
subscription is permitted. SYN+ must use its own historical synthetic
price bars. Its displayed spot-volume alias may be shown to users but
must not produce synthetic volume-confidence credits.

## 3. Pure Z1 module signatures (suggested)

```ts
type Side = 'BUY' | 'SELL';
type ZoneStatus =
  | 'PIVOT_PENDING' | 'CANDIDATE' | 'CONFIRMED_ACTIVE'
  | 'RETESTED' | 'WEAKENED' | 'INVALIDATED' | 'EXPIRED';

interface FinalizedBar {
  symbol: string;
  timeframe: string;
  openTimeMs: number;
  closeTimeMs: number;
  open: number; high: number; low: number; close: number;
  final: true;
  quality: string;
  revision: number;
}

interface ZoneContext {
  /** Completed higher-timeframe candles joined at/before local as-of time. */
  higherTimeframes: ReadonlyMap<string, readonly FinalizedBar[]>;
  contextStatus: 'CONFIRMED' | 'CONTRADICTED' | 'UNAVAILABLE';
}

interface ZoneEvent {
  id: string;
  symbol: string;
  timeframe: string;
  side: Side;
  lowerPrice: number;
  upperPrice: number;
  originTimeMs: number;
  detectedAtMs: number;
  confirmedAtMs: number | null;
  status: ZoneStatus;
  strengthHeuristic: number | null;
  htfStatus: ZoneContext['contextStatus'];
  profileHash: string;
  algorithmVersion: 'smart-zones-v1';
}

interface DetectorResult {
  events: readonly ZoneEvent[];
  activeZones: readonly ZoneEvent[];
  diagnostics: {
    rejectedBars: number;
    candidates: number;
    unavailableHTF: number;
    correctedRevisions: number;
  };
}

function detectSmartZones(
  bars: readonly FinalizedBar[],
  context: ZoneContext,
  immutableProfile: SmartZoneProfile
): DetectorResult;
```

These are **interface targets**, not code already present in QNeXT.

## 4. Non-repainting event-time rules

- Pivot with `(left,right) = (3,3)`: pivot at index `i` cannot be
  detected before the **close** of bar `i+3`.
- A candidate is **not** a BUY/SELL alert. Only a later finalized
  displacement + known-structure-break can confirm it.
- Confirmation time is the confirming bar's close, **never** pivot origin.
- Only HTF bars whose completed close time is `<= confirmation time` are
  eligible. Current, still-forming HTF bars cannot confirm a lower frame.
- If there is no valid higher context (including 1M), show
  `LOCAL_ONLY_UNVERIFIED`, do not label it "MTF confirmed".
- A score such as 78/100 is an explicitly **heuristic strength** with
  reason codes. No win-rate or profit claim is implied.
- A two-close invalidation requires actual finalized close events;
  intrabar wicks can be logged as sweeps but not retroactively treated as closes.
- Dedupe using a stable key of (symbol, timeframe, side, pivot-origin timestamp,
  algorithm version), not `barIndex`; zone ranks may change, IDs never do.

## 5. Strict test suite

| Fixture | Expected assertion |
| --- | --- |
| Small series (<50 finalized bars) | no confirmed zone; "insufficient_history" |
| Early potential pivot | invisible until 3 later finalized bars close |
| Equal low/high cluster | deterministic tie resolution; one stable pivot |
| Displacement without valid structure break | candidate not confirmed |
| Confirmed candidate | confirmedAt > detectedAt >= pivot close |
| Delayed multi-frame close | 15s cannot borrow still-forming 1m confirmation |
| Missing 1m/5m context | explicitly unverified; never "MTF confirmed" |
| 1M with no higher frame | locally informative only, not falsely confirmed |
| Forming last candle | event stream unchanged |
| Same data batch vs sequential | byte-for-byte equal ordered event streams |
| Prefix isolation at every confirmed bar | prefix results equal the corresponding full-run prefix |
| Revision to a historical finalized candle | correction is auditable; stable versioned output |
| Weekend/holiday/session boundary | no invented cross-session candle |
| NIFTY-SYN+ alias volume | no synthetic-volume-based strength credit |
| Duplicate candles and out-of-order updates | deterministic idempotent state |
| Wide wick / bad prices / failed quality | quarantine or reject; never fabricated zones |
| Retest + two-close invalidation | no invalidation from a single forming close |
| 12,000-bar history prepend | bounded memory, stable IDs, no duplicate zones |
| Disabled-by-default | zero detector work and no new historical/WS subscriptions |
| NIFTY, SYN, SYN+, stock, future, option | same engine, instrument-appropriate parameters |

Golden fixtures are immutable after certification. Any future detector
revision changes the algorithm version/profile hash, creates updated
goldens for **new version**, and retains old fixtures for rollback.

## 6. Performance and quality measurement

Use staging first. Benchmark 1,500 and 12,000 bars with 1, 3, and 10
visible charts, and separate history-fetch, detector, and renderer costs.
Record p50/p95/p99 compute durations, bytes/caches, and UI responsiveness.

No claims that Smart Zones improve trading performance until historical
walk-forward and untouched holdout sessions establish precision and
false-positive rates **after** confirmation timestamps. Keep the decision
whether to display a zone distinct from whether to enter a trade.

Availability and resource correctness are a hard gate:
- No change in canonical candle timestamps/prices/volume/persistence.
- No additional Upstox subscriptions from zone calculation.
- No browser memory growth across chart open/close cycles.
- Active bar update must not mutate prior confirmed zone history except
  for auditable source-data corrections.

## 7. Versioned configuration, release and rollback

- Commit the example profile with `enabled_by_default:false` and no
  live admin activation as a **template**, not an active runtime file.
- In Z4 add immutable private profile versions, `sha256`, an atomic
  active revision pointer, authenticated Admin API, optimistic
  `expected_revision`, audit entries and explicit rollback endpoint.
- Stage a new immutable version; run historical and shadow validation.
  Promote only by an authenticated operator action.
- On rollback, point to the previous profile, invalidate zone-result
  caches keyed by profile SHA, recompute from identical immutable candles.
  Never roll back or rewrite canonical market data.
- Keep **no automatic parameter promotion** in selected adaptive mode;
  its volatility-based adjustment follows fixed rule coefficients.

## 8. Acceptance / decision gate

Z1 is DONE only when all golden fixtures, streaming/batch parity,
prefix isolation, session/HTF alignment and provenance tests pass in CI
and the pure detector has **zero imports from market core/network/UI**.

Z2 follows as a separately reviewable staging change, still **OFF**
by default. It needs Vela transient-box parity tests. Z3 Lightweight,
Z4 config rollback, Z5 multi-session backtests and Z6 optional Lab
features remain independent gates.

Any fallback or insufficient data must be explicit in metadata and UI.
There is no silent switch from "multi-timeframe confirmed" to local-only,
and no automatic trade placement.
