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
