# QNext Vela 0.9.0 / PineTS 0.2.18 upgrade gate

## Pinned package set (published versions)

- `@luxalgo/vela`: 0.8.4 → **0.9.0** (renderer, workspace, drawing and API support).
- `@luxalgo/vela-pinets`: 0.2.16 → **0.2.18** (upgrades both `PineEngine` and `PineWorkerEngine`; includes marker-offset fixes from 0.2.17).
- `pinets`: 0.11.0 → **0.12.1** (in-process Pine runtime; the worker bundle uses its OWN build-time PineTS, per upstream).
- `lightweight-charts`: 5.2.1 (unchanged fallback).

The newer 0.3.0 upstream addon source requires pinets >=0.13.0 and is NOT selected until both are confirmed published and CI-tested. Do not install directly from upstream main.

## Execution paths

Production default: `PineWorkerEngine` on a Web Worker (`pineEngine: 'worker'`).
Controlled diagnostic: set `window.__QNEXT_CONFIG__.pineEngine = 'main'` before startup to run `PineEngine` on the chart thread. Never enable this diagnostic on normal public sessions; it can block the UI on expensive scripts. Both implement the same Pine ScriptingEngine port, and both share QNext market data through Vela.

## CI and staging gates

1. `npm install --no-audit --no-fund && npm ls --depth=0` (from `apps/workspace`).
2. `npm run typecheck && npm test && npm run vela:parity && npm run build`.
3. Q2 Workspace and other required PR checks green. CI **does not prove staging**.
4. Browser smoke test Vela boot, zoom and viewport stability; theme; timeframe/style dropdowns; drawings/saved layouts; volume on NIFTY/SYN/SYN+; 15s, 30s, 1m, 5m and 15m; inactivity resume; reconnect/gap repair.
5. Live Pine marker test: run the SAME Buy/Sell Pine script at the SAME symbol/timeframe on TradingView and staging. Trace (a) REST/WSS final/revision + OHLCV, (b) bars handed to the engine, (c) Pine output, (d) model marker render, including `plotshape` with and without `offset`, `barstate.isconfirmed`, MTF `request.security`, and 15-second/1-minute boundaries. Confirm markers appear without refresh and do not backdate alerts. Compare after reload using identical closed candles.
6. Run this live parity first in worker mode; when output differs, repeat in explicit main-thread diagnostic mode to isolate worker transport from Pine semantics. Avoid assertions that a candle must signal before its scripted confirmation rule.
7. Delayed finalized-bar volume/price revisions still need separate correction handling: current Vela 0.9.0 `applyBar` rejects bars older than the latest. This upgrade does NOT fix that reconciliation gap.
8. Confirm `digiops-release` artifact built from the merged `main` commit, staging deployment succeeded, and the deployed chart serves the tested versions. Keep the prior working artifact available for rollback.

## Safety and licensing

Stage first. Preserve drawings/layout state and all existing Go candle/volume authority; do not modify market-core as part of this dependency upgrade. The `@luxalgo/vela-pinets` addon and PineTS are AGPL-3.0-only; legal review remains required for closed-source/network deployment. Vela core remains Apache-2.0.
