# QNext chart-stack upgrade — Vela v0.10.14 / Lightweight Charts v5.3.0

## Versions and decision (22 November 2026)

| Dependency | Prior | Candidate | Decision |
| --- | --- | --- | --- |
| `@luxalgo/vela` | 0.10.6 | **0.10.14** | Upgrade |
| `@luxalgo/vela-pinets` | 0.3.4 | **0.3.4** | Retain; newest verified published compatible addon |
| `pinets` | 0.13.1 | **0.13.1** | Retain; newest verified published npm version |
| `lightweight-charts` | 5.2.1 | **5.3.0** | Upgrade |

The official LuxAlgo/Vela release v0.10.14 was published November 20,
2026. The addon `@luxalgo/vela-pinets` peers
`@luxalgo/vela: ^0.10.0` and `pinets >=0.13.0`. QNext stays on
`0.3.4` and `0.13.1` rather than inventing later npm versions.

## Vela release differences that matter for QNext

- **Indicator model reruns:** fixed fill/background/color-bar divergence
  after loading more history, so fills can continue through live bars.
- **Countdown:** improved candle-close readout on slowly set device clocks.
- **Split charts:** fixed maximized chart behavior to avoid unused
  full-screen canvas memory on iOS Safari.
- **Drawings sync:** BREAKING behavioral change; drawings now sync only
  between charts showing the *same symbol*, instead of across every
  linked chart, regardless of symbol.
- **Native fullscreen:** QNext opts into Vela's new desktop
  `fullscreen` topbar action and Alt+Enter shortcut, retaining QNext
  alerts, Screen Awake, Zoom, Data Window and Screenshot actions.

Lightweight Charts 5.3.0 includes historical-update and panning fixes,
but also validates business-day string formatting more strictly. QNext
must provide canonical timestamps / ISO zero-padded business days.

## Code and release checks

- Exact dependency pins and official LuxAlgo fullscreen method validated
  by `npm run vela:parity`.
- Vela, Vela PineTS, Pine Worker streaming capability and Lightweight
  Charts v5 series API tested at runtime by the parity script.
- Existing Vite typecheck/build and all workspace Vitest tests continue:
  two-section symbols; 5s/15s/1m/2m/5m history; late/backfilled candle
  revisions; live Buy/Sell Pine label/marker telemetry; confirmed MTF
  recompute; provider duplicate/out-of-order handling; viewport/history
  extension; tab resume and Lightweight chart mapping.
- A new upgrade contract guards dependency selection, new fullscreen
  action and QNext market/Pine wiring.
- No Go Market Core, stock/NSE 1m polling, Upstox WebSocket,
  SYN+ futures volume proxy or backend storage changes.

## Staging certification is mandatory

CI success is not proof of chart parity. Install the candidate DigiOps
package **on staging only**, record its release SHA and test authenticated
Vela and Lightweight chart engines against the previous release:

1. Load the 13 core index/SYN symbols and configured NSE historical 1m
   group; verify picker labels, load and timeframe transitions. Confirm
   no synthetic group-root is interpreted as a market instrument.
2. Test 5s, 15s, 30s, 1m, 2m and 5m candles on available markets.
   Verify incoming finalized candles, late revisions and repeated chart
   refresh at live/open-market and historical/closed-market times.
3. Test Pine v6 higher-timeframe indicators, worker/main engine parity,
   markers appearing *live* before refresh, fill/bgcolor/plotshape across
   history pagination and a hard refresh. Compare with TradingView
   on the same finalized timestamp; no invented parity claims.
4. Verify NIFTY SYN+ volume during a real market session. Index
   cash-index volume remains unknown, and the history-only NSE stock
   list must not subscribe live to extra symbols.
5. Test split view drawing sync separately for same/different symbols,
   maximization, iOS memory, Vela fullscreen Alt+Enter and Escape,
   reduced zoom, topbar/search and right-side plugin panels.
6. Test Lightweight Charts 5.3.0 for historical point insertion,
   time-scale stability, daily timestamp formatting and interval switches.
7. Capture before/after JS bundle sizes and actual browser loading
   performance (LCP/INP/CLS, heap) on equal devices/networks. Do not
   claim "superfast" without measurements.

Rollback: revert to the previous known-good DigiOps staging artifact
if any chart/Pine/live-signal contract fails. Release build success alone
does **not** authorize a production deployment.
