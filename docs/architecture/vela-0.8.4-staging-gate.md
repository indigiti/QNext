# Vela 0.8.4 staging upgrade gate

## Scope and pinned dependencies
- `@luxalgo/vela`: 0.7.7 → 0.8.4
- `@luxalgo/vela-pinets`: 0.2.13 → 0.2.16
- `pinets`: 0.10.0 → 0.11.0
- `lightweight-charts`: retain 5.2.1 as fallback
- Go Market Core, canonical candle formation, market data APIs, and persistence are unchanged.
- `qnext-workspace-v2` persistence key is unchanged; saved layout, drawings and indicators must migrate cleanly.

## Automatic acceptance gates
- Install dependencies cleanly and validate graph: `npm install --no-audit --no-fund && npm ls --depth=0` from `apps/workspace`.
- `npm run typecheck`, `npm test`, `npm run vela:parity`, `npm run build`.
- GitHub Actions `Q2 Workspace` succeeds for the upgrade PR, including provider and market-core tests.
- Release workflow `qnext-release` packages static workspace and keeps all existing PHP/Go validation.
- **No promotion based solely on the static build.**

## Mandatory interactive staging checks (market data must remain authoritative)
1. Vela boot, canvas sizing, light/dark mode, topbar actions and mobile overflow; check no overlapping controls.
2. Symbols `NSE:NIFTY`, NIFTY-SYN and NIFTY-SYN+ (matching active catalog names), including 15s, 30s, 1m, 3m, 5m, 15m, 1h and 1D.
3. Live candles update within their interval: timestamp, OHLC and volume agree with Go canonical REST/WebSocket data; never manufacture bars to fill an exchange-closed session.
4. Switch symbols/timeframes repeatedly, inspect backfill while panning, check viewport preservation and bounded history loads.
5. Hide tab for >2 seconds, restore focus, and verify chart-resume recovery; simulate socket loss/reconnect and REST fallback with no duplicates, gaps or stale data after reconciliation.
6. Existing Pine v6 indicators and custom QNeXT indicators, candle/bar colors, persistence, and runtime script error recovery.
7. Drawings, edits, synced state, save/reload and rollback of stored workspace state; inspect 0.8.x drawing settings and FVG/OB/Liquidity tools.
8. Replay and multi-chart synchronization (if enabled) must not leak future bars or execute live trading signals.
9. Compare Vela against the separate Lightweight Charts engine as fallback for identical input candles and volume.
10. Examine devtools errors, memory/leaks after repeated chart changes, frame rate and live-feed latency versus the previous staging build.

## Promotion and rollback
- Deployment target: staging only, via current DigiOps `digiops-release` process after CI and manual verification.
- Capture staged commit/artifact ID, smoke-test results and monitor the next open Indian equity session.
- If a failure is detected, redeploy the last known-good release (Vela 0.7.7/PineTS 0.2.13) and restore the preceding artifact, without altering Go market data.
- Do not overwrite or delete user saved workspace state. Verify backward compatibility of persisted state before allowing rollback.
- A successful GitHub commit or CI pass **does not** mean staging was deployed or the browser checks passed.

## License review
Vela core is Apache-2.0; the Vela PineTS addon is AGPL-3.0-only. Confirm deployment and distribution obligations with legal review before production promotion.
