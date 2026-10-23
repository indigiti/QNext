# Vela / Pine live Buy-Sell signal check (post release #117)

## Known compatibility boundary

QNext Market Core sends WSS candle updates with `final` and `revision`. QNextProvider delivers those changes, but Vela 0.9.0's private `EngineOrchestrator.applyBar` drops a bar when its time is older than the newest bar. PineWorkerEngine 0.2.18 then sees only the latest bar and newer bars. A revised prior candle can therefore differ from an indicator's historical calculation after refreshing.

This check does NOT yet prove that the user's particular script mismatch comes from late revisions; different Pine Script `barstate`, `plotshape offset`, `request.security`, volume, or calculation semantics remain possible.

## Opt-in, bounded local trace

Append `?pine_trace=1` to the staging chart URL **before loading the page**. The chart runs normally, with no refresh loops or automatic recalculation added. In browser DevTools console:

- `window.__QNEXT_PINE_TRACE__.summary()`
- `window.__QNEXT_PINE_TRACE__.snapshot()`

Events are limited to the newest 300 and never sent to a backend. The trace captures incoming provider bars (including revision), bars Vela actually accepts, `script:run` events including cause, and indicator errors. It does not collect source code or credentials. Tracing is off by default because Vela script-run observation may incur engine-context work.

A WSS `provider-bar` with `olderThanNewest: true`, lacking a matching later `vela-bar` for that bar time, confirms the known discarded-revision path. The missing Pine `bar` run alone is not sufficient to attribute the bug to revision loss.

## Live validation

Run the same Pine script on TradingView and QNext for an identical symbol, timeframe, inputs and finalized OHLCV. Record when signal first appears (intrabar vs confirmed); compare after the next candle opens and then after historical reload. Test 15s / 1m NIFTY / SYN / SYN+ and MTF when relevant.

Only after the cause is isolated should upstream Vela/Pine be patched. **Do not** simulate a fix by calling `setMarket({bars})`: Vela only extends the oldest edge, leaving recently corrected bars untouched. Forced market switches alter viewport/indicators and are not safe for production. For an enduring fix, add a supported revision-aware chart-core upsert + bounded invalidation/recompute from the earliest changed candle, with prior revision ordering and no duplicate alerts.

Run existing staging-first CI and keep #117 available for rollback.


## Post-#118 engine-output observation

A `script:run` event alone does **not** show whether `plotshape` produced a Buy/Sell marker. In a diagnostic-only build, the `pine-model` event intercepts upstream `ScriptingEngine.onModel` before Vela applies the model. It captures only compact marker geometry from `model.labels` (plotshape/plotchar/plotarrow or label.new) and `model.series[kind='markers']`, with bar-index labels resolved against the current model anchor. No Pine source, account secrets or historical candle dump are retained. Only changes are logged, max 300 events.

Decision:
- Revised provider bar not emitted as a matching Vela bar: candle-revision gap (upstream Vela reconciler).
- Vela accepts a finalized candle but no subsequent Pine execution/model: worker scheduling or execution gap.
- Pine model contains the signal marker at the expected candle timestamp, but chart is visually missing it: renderer/mount/visibility issue.
- Pine executes but model has no signal when TradingView does: compare exact script inputs and OHLCV, barstate confirmation, `request.security` and offset before making changes.

This trace is passive. It cannot, by itself, verify a screenshot or TradingView behavior: side-by-side timestamp evidence is still required. Tests use mocked engine outputs, not a real exchange feed.
