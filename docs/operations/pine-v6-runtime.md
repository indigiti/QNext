# Pine Script v6 Runtime

QNext supports file-backed Pine Script v6 custom indicators and strategies in the chart workspace.

## Runtime

The workspace registers `@luxalgo/vela-pinets@0.2.13` with Vela `0.7.7` using `PineWorkerEngine`. Pine source therefore executes off the browser UI thread while consuming the same canonical QNext bars used by native indicators.

The runtime stack is:

`QNext bars -> VelaWorkspace -> PineWorkerEngine -> PineTS -> plots/trades`

Pine scripts do not execute inside Go Market Core, PHP Ops, or the broker/feed process.

## Admin workflow

Open **Admin -> Custom Indicators** and choose **+ Add Pine Script**.

Required fields:

- Name
- ID
- Kind: `Pine Script v6`
- Source beginning with `//@version=6`
- An `indicator(...)` or `strategy(...)` declaration

Enabled scripts are published by `/qnext/api/v1/indicators/` with `language: "pine"` and appear under **Indicators -> QNext**. Editing, enabling, disabling, and deleting are file-backed operations and do not restart Market Core.

The admin API caps a Pine source file at 256 KiB.

## Licensing boundary

`@luxalgo/vela-pinets` and `pinets` are licensed **AGPL-3.0-only**. Vela itself remains Apache-2.0.

QNext deliberately keeps the Pine runtime dependency in `apps/workspace`; Market Core and the native QNext indicator implementation do not import PineTS. Deployments that distribute or provide network access to the AGPL-covered runtime should satisfy the applicable AGPL source-offer and notice obligations.

Upstream source:

- https://github.com/LuxAlgo/Vela-pinets
- https://github.com/LuxAlgo/PineTS

## Validation

CI verifies:

- Vela and PineWorkerEngine runtime parity
- workspace TypeScript build
- Pine manifest publication from the deployed public/private directory layout
- Ops API Pine v6 persistence and rejection of non-v6 source

Runtime compilation errors in a pasted Pine script are surfaced by the Pine engine when the user adds/runs the script on a chart.

## MTF refresh consistency policy

The workspace applies a fresh-snapshot execution policy to Pine scripts with
`// @qnext-confirmed-mtf-recompute` and to legacy admin-managed copies of
`Trinity MidPoint ATR Ribbon` containing a `request.security()` call. Other
scripts continue normal incremental streaming. User-selected timeframes and
Pine inputs are passed through unchanged.

Snapshots run on a new chart candle, changes to the immediately preceding
candle, changed history edges/count, and history completion; the upstream
engine handles input updates and coalesces in-flight worker runs. Ordinary
forming-candle ticks do not trigger full-history recalculations. Consequently
the opted-in ribbon updates at chart-bar cadence rather than tick cadence.

This policy does not rewrite Pine `request.security()` lookahead or offsets,
nor guarantee non-repainting semantics for developing HTF candles. Compare
closed-bar plots with identical input settings, warm-up history, and OHLCV
revisions before declaring refresh parity. The live deployed Pine script is
admin-managed and may require its own confirmed-HTF expression correction.
