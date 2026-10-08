# QNext Intelligence Lab — isolated ML research plane

## Purpose

Intelligence Lab is the experimental research plane for indicator-driven ML work. It is deliberately separated from the production QNext Intelligence runtime so model experiments, feature changes, parameter sweeps and future ML libraries cannot disturb the running chart, market-data path or current production model.

```text
Go Market Core
      │ canonical finalized bars
      ├──────────────────────────────► Production QNext
      │
      └──────────────────────────────► Intelligence Lab
                                         │
                                         ├─ chart-indicator feature snapshots
                                         ├─ model experiments
                                         ├─ historical / walk-forward tests
                                         ├─ untouched holdout
                                         └─ shadow-live certification
```

## Hard boundary

Production intelligence storage remains:

```text
storage/intelligence/
```

Lab artifacts live under:

```text
storage/intelligence-lab/
  experiments/<experiment-id>/
    manifest.json
    results/
      backtest.json
      shadow.json
  events.jsonl
```

The lab registry rejects configuration that points at the production storage root or any descendant of it.

Lab code does not write `storage/intelligence/models/production.json`, does not promote production models and does not participate in live order execution.

## Lifecycle

The v1 lifecycle is intentionally one-way:

```text
EXPERIMENT
    ↓ passing backtest gate
BACKTESTED
    ↓ explicit shadow start
SHADOW
    ↓ passing shadow gate
CERTIFIED
    ↓
eligible for a later, separate production-import workflow

Any active state
    ↓
RETIRED
```

A failed backtest or shadow gate records the result but does not advance lifecycle state.

There is deliberately no `PRODUCTION` state in the Lab registry. Moving a certified model into production will be implemented as a separate hash-locked import/promotion operation so Lab code itself can never silently activate a model.

## Chart-driven indicator context

The intended experiment identity includes:

- instrument
- timeframe
- enabled chart indicator IDs
- indicator configuration hash
- feature schema version

Therefore:

```text
NIFTY 1m + EMA + OB + SYN+
```

and

```text
NIFTY 1m + EMA + OB + SYN+ + RSI
```

are separate intelligence contexts.

Indicator BUY/SELL labels are not required. The feature collector will expose normalized numeric/state features from the indicators enabled on the chart.

## First implementation slice

The first slice adds:

- isolated file-backed experiment registry
- deterministic experiment IDs
- immutable experiment manifests
- immutable backtest/shadow result slots
- lifecycle transition enforcement
- append-only audit events
- storage-isolation checks
- unit tests for isolation, gate failure and lifecycle order

The next slice will add chart-clone experiment creation, feature contracts and the historical evaluation runner. ML libraries (scikit-learn / LightGBM) remain deferred until the lab boundary and leakage-safe dataset flow are certified.


## Chart-to-Lab workflow

The active Vela chart now exposes a host-level **Send to Lab** action. Vela remains the indicator execution authority; the bridge only reads the active cell and its visible indicator handles.

The capture includes:

- active symbol and timeframe
- only visible/enabled indicator instances
- script source hash and persisted input overrides
- deterministic indicator configuration hash
- time-indexed numeric plot features when exposed by the engine
- current numeric variables for diagnostics

The workspace first submits the immutable snapshot to the authenticated Admin Lab endpoint. If the Admin session is not available, it keeps a browser-local pending handoff and opens the Admin surface; the authenticated operator can then import it.

```text
Active Vela chart
      ↓ Send to Lab
read-only indicator snapshot
      ↓
Admin-authenticated Lab import queue
      ↓
storage/intelligence-lab/experiments/<id>
```

No chart setting, indicator input, production model or live strategy is modified by this action.

## First Lab backtest runner

The initial Lab model is intentionally the existing deterministic QNext ridge learner, reused as a benchmark before external ML dependencies are introduced.

The Lab worker:

1. loads the immutable chart snapshot;
2. requests the matching canonical bars from Market Core;
3. aligns indicator features only to their finalized origin bar;
4. creates future return / MFE / MAE / direction outcomes from later bars only;
5. performs the existing chronological train / validation / untouched-test split;
6. writes the candidate only under the experiment's Lab model directory;
7. records a BACKTEST evaluation and advances to BACKTESTED only when its gates pass.

The Lab worker has no production-promotion action.

## Feature coverage rule

The first runner learns only from historical numeric feature series that the chart runtime exposes. Current-bar variables are retained for diagnostics but are not retroactively fabricated into historical rows.

An indicator that renders only drawing objects (for example boxes/labels) and exposes no numeric historical series is therefore not silently treated as a useful ML feature. A follow-on indicator feature contract will expose drawing/state semantics explicitly (zone prices, distances, ages, structure state, etc.) before such information is admitted to model training.


## Indicator feature contracts (v2)

Chart snapshot schema version `qnext-chart-indicators-v2` records feature coverage per enabled indicator.

Each indicator descriptor now reports:

- historical feature names available to model training;
- current-only numeric features available for diagnostics;
- source/configuration hashes and persisted Vela input overrides.

Certification is fail-closed: every enabled indicator must contribute at least one stable historical feature after coverage/variance filtering. The feature selector reserves representation for each enabled indicator before applying the global feature cap.

Adaptive EMA exposes an explicit historical contract including EMA, price, ATR, standard deviation, trend, EMA distance, EMA slope, ATR percent and standard-deviation percent.

For Pine scripts that render drawings but expose no historical plot series, **Send to Lab** may run a separate invisible Vela/PineWorker clone over canonical Market Core bars. QNext auto-discovers safe top-level scalar Pine state and appends hidden plots only to that Lab clone. The visible chart source and runtime instance are never modified.

Examples of auto-contract state include structure/trend integers, last swing levels and CHoCH booleans. Arrays, boxes, labels and local block variables are not rewritten into unsafe guesses. If no safe historical state can be extracted, the experiment can still be imported for diagnostics but cannot pass the backtest gate.

## Lab ML tournament

The deterministic Ridge learner remains the mandatory first benchmark and fail-safe.

Optional Lab-only dependencies:

- `scikit-learn==1.9.1`
- `lightgbm==4.7.0`

The tournament runs:

```text
chronological dataset
      ↓
60% train
      ↓
20% validation ── choose family/parameters/confidence threshold
      ↓
20% untouched test ── compare selected challenger against Ridge
```

Current challengers:

- scikit-learn logistic classifier with standardized features;
- LightGBM classifier with deterministic single-threaded training settings.

The selected ML family must improve the validation objective before it is allowed to see the untouched test window. It then must satisfy return, accuracy and drawdown gates versus the Ridge benchmark. Any missing dependency or ML runtime/training error falls back to Ridge; it does not fail the Lab backtest or affect production Intelligence.

Probability output is explicitly marked **not calibrated** at this stage. A displayed percentage must not be treated as a literal historical win probability until a leakage-safe calibration stage is added.

The Lab persists the tournament winner, model hash, Ridge candidate, optional ML candidate, dependency versions, indicator feature coverage and failure/fallback reason. Admin shows the selected algorithm after each backtest.

## ML deployment boundary

CI installs and executes the optional ML stack to certify the code path. The normal QNext release does not yet install NumPy/SciPy/scikit-learn/LightGBM into the live host Python runtime.

This is intentional. Until an isolated Lab ML environment is provisioned, deployed Lab backtests remain fully functional with the deterministic Ridge benchmark. No compiled ML dependency is introduced into the Go Market Core, chart runtime or production Intelligence path.

## Isolated Lab ML runtime

The deployed Lab ML stack is isolated from the production Intelligence interpreter.

The release artifact contains:

```text
private/
  intelligence/
    qnext_intelligence/          # dependency-light production/Lab source
  intelligence-lab-wheels/
    requirements.lock
    wheelhouse.sha256
    *.whl
  deploy/
    qnext-intelligence-lab-python
```

The wheelhouse is built on Python 3.11 with binary-only packages and SHA-256 verification. It contains the pinned Lab ML dependencies and their transitive wheels, so the server does not need network access to provision ML.

The first **backtest** that needs ML lazily creates:

```text
<private-root>/runtime/intelligence-lab-venv/
```

The virtual environment is keyed to the immutable wheelhouse fingerprint. A matching environment is reused; a changed wheelhouse is staged into a new environment and atomically swapped only after dependency verification succeeds.

Simple chart-snapshot imports do not trigger ML provisioning.

Runtime selection is:

```text
Lab backtest
   ↓
isolated venv READY?
   ├─ yes → run Ridge + scikit-learn + LightGBM tournament
   └─ no  → system Python → deterministic Ridge fallback
```

Failure to create or verify the isolated environment writes a Lab runtime status record and does not modify the production Intelligence environment. Admin exposes the runtime state (`READY`, `NEEDS_BOOTSTRAP`, `UNAVAILABLE`, `FAILED`, or `UNKNOWN`).

CI exercises both paths:

- missing wheelhouse → explicit Ridge-safe fallback status;
- real local wheelhouse → isolated venv creation, scikit-learn/LightGBM imports, version checks and idempotent reuse.

The live Go Market Core, Vela workspace, production Intelligence model pointer and production Python dependency set are unaffected.

## Recommendation intelligence

A passing Lab backtest can now emit an immutable recommendation artifact:

```text
BUY / SELL / NO_TRADE
calibrated class probabilities
entry price
Target 1
Target 2
invalidation
expected side return
expected horizon in bars
```

The recommendation remains Lab-only. It is not sent to broker execution, Strategy Paper, production Intelligence, or the chart order path.

### Probability calibration

The selected Ridge/scikit-learn/LightGBM model first produces raw class probabilities. The Lab fits a single temperature-scaling parameter on the chronological validation window only. The untouched test window is then used to report calibrated log loss, Brier score, accuracy, coverage, return and drawdown.

The action threshold is also chosen on validation data. A recommendation becomes `NO_TRADE` when directional probability is below the validation-selected threshold or the calibrated FLAT probability dominates.

### Learned targets and invalidation

For every labelled historical origin, the Lab stores the full future high/low excursion path across the configured horizon. Target and invalidation profiles are learned separately for BUY and SELL from validation-period recommendations:

- Target 1: median favorable excursion;
- Target 2: 75th-percentile favorable excursion;
- Invalidation: 75th-percentile adverse excursion;
- Expected return: median side-normalized terminal return;
- Expected horizon: median bars to Target 1, falling back to favorable-peak timing when Target 1 is not reached often enough.

The untouched test window reports Target-1-before-invalidation, Target-2-before-invalidation and invalidation-before-Target-1 rates.

Intrabar ordering is deliberately conservative. If a single OHLC bar can touch both the target and invalidation level, the evaluator records invalidation first because tick order is unknown.

Recommendation artifacts include experiment ID, model algorithm/hash, feature schema version, calibration method, decision threshold, latest finalized-bar timestamp and recommendation ID for auditability.

## Shadow-Live validation

A `BACKTESTED` experiment can be advanced explicitly to `SHADOW`. This does not change the production model, broker execution, chart order path or Strategy Paper.

Shadow-Live evidence is generated only from new finalized canonical bars after the experiment enters `SHADOW`.

```text
Vela chart + enabled indicator context
        ↓
new canonical finalized bar
        ↓
fresh chart feature rows (same config hash/schema)
        ↓
protected Shadow-Live inbox
        ↓
selected Lab model + stored calibration policy
        ↓
private BUY / SELL / NO_TRADE observation
        ↓
wait configured future horizon
        ↓
canonical outcome + MFE/MAE + target/stop ordering
        ↓
Shadow summary
```

The browser observer is dormant unless an experiment is already in `SHADOW` and the browser has an authenticated Admin session. Lightweight Charts are not changed; Shadow feature capture currently follows the Vela indicator-contract path.

Each Shadow observation is immutable and keyed by experiment, finalized bar time, model hash and feature snapshot hash. Browser retries are idempotent. Pre-Shadow bars are ignored so historical replay cannot be counted as live evidence.

Observations use a separate append-only inbox rather than the one-action Admin queue. This allows sub-minute bars to accumulate without being dropped between supervisor cycles. The supervisor drains up to 50 observations per cycle and retains failed files for retry.

### Explicit certification gates

`CERTIFIED` is never automatic. An Admin must click **Evaluate certification** while the experiment is in `SHADOW`.

The default live gates are:

- at least 30 completed Shadow outcomes;
- live coverage at least 10%;
- accuracy no more than 5 percentage points below the untouched backtest;
- average strategy return no more than 0.2 percentage points below the untouched backtest;
- max drawdown no more than 2 percentage points above the untouched backtest;
- average multiclass Brier score no worse than 0.35;
- Target-1-before-invalidation at least 30% when target evidence exists.

The Shadow horizon is locked to the original backtest horizon. It cannot be changed when Shadow starts.

Failed certification attempts remain in `SHADOW` and are retryable as more live observations mature. Every certification evaluation remains immutable while `results/shadow.json` points to the latest evaluation.

Certification only changes the Lab lifecycle to `CERTIFIED`. It still does not promote a production model or enable live execution.

## Certified chart advisory

A Lab experiment that passes Shadow-Live certification can publish a read-only chart advisory while remaining isolated from production execution.

```text
CERTIFIED Lab experiment
      ↓
matching Vela chart context
      ↓
new canonical finalized bar
      ↓
protected Admin-session scoring request
      ↓
selected certified model + stored calibration policy
      ↓
immutable certified advisory artifact
      ↓
sanitized read-only workspace endpoint
      ↓
Certified Intelligence card
```

### Exact context contract

The chart displays an advisory only when all of the following match exactly:

- canonical instrument ID;
- timeframe;
- feature schema version;
- enabled indicator configuration hash.

The configuration hash includes enabled indicator instance identity, script/native source hash, language/type and resolved inputs. If an indicator is added, removed, hidden or reconfigured, the advisory disappears rather than falling back to a different model context.

A lightweight context fingerprint path computes this hash without repeatedly extracting full indicator history.

### Publishing boundary

Live scoring remains protected by the Admin HttpOnly session. An authenticated Vela chart submits finalized feature rows to the existing append-only Intelligence live inbox. `SHADOW` targets continue producing outcome evidence; `CERTIFIED` targets switch to advisory-only scoring.

Certified advisory scoring does not update Shadow metrics, place orders, change Strategy Lab, or modify the production Intelligence model pointer.

### Public chart payload

The workspace endpoint returns only sanitized fields from `CERTIFIED` experiments:

- decision (`BUY`, `SELL`, `NO_TRADE`);
- calibrated probabilities;
- entry, Target 1, Target 2 and invalidation;
- expected side return and horizon;
- instrument/timeframe/context hashes;
- model algorithm/hash and recommendation-policy ID;
- certification/advisory timestamps.

It never exposes feature vectors, candidate artifacts, coefficients, model payloads, datasets, Admin tokens or execution instructions.

The Vela chart shows the result in a compact **Certified Intelligence** card labelled **Advisory only · no execution**. Advisories older than a bounded timeframe window are visibly marked `STALE`.

Lightweight Charts do not display certified advisories yet because they do not currently prove parity with the Vela enabled-indicator context.
