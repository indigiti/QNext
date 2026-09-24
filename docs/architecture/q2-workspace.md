# Q2 — QNext Workspace

Status: **CERTIFIED**

Q2 closes the first browser-facing vertical slice:

```text
QNext Market Core
  ├── canonical symbols
  ├── versioned market calendars
  ├── canonical history
  └── realtime bar stream
          ↓
     QNextProvider
          ↓
     VelaWorkspace
          ↓
 symbols + timeframes + indicators + drawings + persistence
```

## Runtime ownership

The production browser app is a static build.

- TypeScript and Vite are build-time tooling only.
- There is no production Node.js server.
- The Vela workspace is emitted under the configured `/qnext/` base.
- Go remains the realtime market-data authority.
- The workspace consumes only QNext canonical REST/WebSocket contracts.

## QNextProvider

`apps/workspace/src/qnext-provider.ts` implements Vela's provider contract:

- `listSymbols()` -> `GET /api/v1/symbols`
- `getBars()` -> `GET /api/v1/bars`
- `getCalendar()` -> `GET /api/v1/calendar`
- `subscribe()` -> `/api/v1/stream` WebSocket

History is normalized, sorted and de-duplicated before it reaches Vela. Live bars use the same canonical bar shape. Reconnects use QNext stream `resume`; a server `resync_required` response falls back to a fresh canonical subscription.

Provider-specific broker keys never enter the browser contract.

## Symbols

The visible Q2 workspace catalog is:

- `NSE:NIFTY` -> canonical market-core instrument `NSE:NIFTY50`
- `QNEXT:NIFTY-SYN` -> canonical market-core instrument `QNEXT:NIFTY-SYN`

Option legs remain registered internally when live market configuration is loaded, but they are not exposed as workspace symbols.

## Market calendar

Both Q2 symbols use calendar `NSE_EQ`.

Version `nse-equities-2026-v1` resolves the NSE regular trading session as 09:15–15:30 Asia/Kolkata and carries the exchange's published 2026 equity holiday closures.

The calendar deliberately fails closed outside its certified `[2026-01-01, 2027-01-01)` window. Annual calendar updates must be versioned rather than silently assuming weekdays are trading days.

The 8 November 2026 Diwali Muhurat special session is not fabricated while its special timing is not represented in this calendar version. A later exchange-timing update should be added as a new calendar version.

Reference: NSE Market Timings & Holidays, 2026 equities.

## Vela workspace

Q2 pins `@luxalgo/vela@0.7.7` and uses `VelaWorkspace` in single-chart mode.

Initial workspace behavior:

- symbol: `NSE:NIFTY`
- timeframe: `1m`
- QNext timeframes: `15s`, `30s`, `1m`, `3m`, `5m`
- live canonical bars enabled
- exchange timezone mode enabled
- persistent workspace document key `qnext-workspace-v1`
- Vela's native indicator picker remains enabled
- no Pine/PineTS runtime is required for Q2

Vela's built-in native indicator catalog therefore remains available without introducing the separately licensed Pine addon.

## Certification

The `Q2 Workspace` workflow must pass both jobs.

### Market Core

- [x] canonical symbol registry remains provider-independent
- [x] deterministic provider authority contract
- [x] visible symbol catalog endpoint
- [x] versioned calendar endpoint
- [x] NSE holiday/weekend certification tests
- [x] full Go race-test regression
- [x] market-core build

### Workspace / Vela

- [x] QNextProvider symbol parity
- [x] QNextProvider history normalization
- [x] QNextProvider calendar parity
- [x] QNextProvider live-stream mapping
- [x] static Vite production build
- [x] Vela package pinned
- [x] Vela constructor/workspace state API runtime parity probe
- [x] release artifact name `digiops-release`
- [x] **Vela runtime/parity PASS in GitHub CI**

Q2 certification is complete: the combined workflow is green.
