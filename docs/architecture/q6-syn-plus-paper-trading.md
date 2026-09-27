# Q6 — NIFTY-SYN+ paper trading

## Purpose

Enable deterministic paper trading on `QNEXT:NIFTY-SYN+` analysis without granting the research synthetic any broker or trading authority.

## Scope

Phase 1 is Strategy Paper only:

`GOOD final SYN+ bar -> strategy -> Q4 PaperEngine -> pending order -> next-bar fill -> paper position/equity/markers`

The paper session is locked to one configured SYN+ instrument and one timeframe. It rejects wrong-symbol, wrong-timeframe, non-final and quality-ineligible bars.

## Safety boundary

The adapter contains no broker client, broker credentials, order-routing interface or live-order promotion path. `broker_execution_enabled` is permanently false in this slice.

SYN+ remains a shadow/research instrument. This feature does not alter `QNEXT:NIFTY-SYN`, Market Core production authority, synthetic promotion rules or canonical market truth.

## Execution semantics

The existing Q4 `PaperEngine` remains authoritative for paper execution:

- signal is produced only from the current final bar;
- an order generated from that signal cannot fill on the same bar;
- fill happens against the next eligible bar under versioned deterministic execution assumptions;
- cash, position, equity, fills and SIGNAL/FILL chart markers remain reproducible;
- `GOOD` and `RECOVERED` quality are eligible; degraded/non-final bars fail closed.

## Default configuration

- instrument: `QNEXT:NIFTY-SYN+`
- timeframe: `1m`
- initial capital: `100000`
- unit size: `1`
- starter strategy: SMA cross (`fast=2`, `slow=3`)

These defaults are for plumbing/certification, not a claim of trading efficacy. Strategy selection and parameters should be supplied by the analysis/optimization layer in later slices.

## Follow-on

1. live-paper event adapter from finalized SYN+ candle stream;
2. persistent paper sessions and performance attribution;
3. Ops Console controls/status and chart marker fan-out;
4. tradable-leg paper mode using the Auto Leg Manager and real option/future quote execution simulation;
5. promotion gates based on deterministic backtest + forward paper evidence.
