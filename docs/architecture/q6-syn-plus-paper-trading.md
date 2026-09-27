# Q6 — NIFTY-SYN+ paper trading

## Purpose

Enable deterministic paper trading on `QNEXT:NIFTY-SYN+` analysis without granting the research synthetic any broker or trading authority.

## Current scope

Strategy Paper is implemented as:

`GOOD/RECOVERED final SYN+ bar -> Strategy Lab -> Q4 PaperEngine -> pending order -> next-bar fill -> paper position/equity/P&L/markers`

The paper session is locked to one configured SYN+ instrument and one timeframe. It rejects wrong-symbol, wrong-timeframe, non-final and quality-ineligible bars.

## Safety boundary

The adapter and runtime contain no broker client, broker credentials, order-routing interface or live-order promotion path. `broker_execution_enabled` is permanently false in this slice.

SYN+ remains a shadow/research instrument. This feature does not alter `QNEXT:NIFTY-SYN`, Market Core production authority, synthetic promotion rules or canonical market truth.

## Execution semantics

The existing Q4 `PaperEngine` remains authoritative for paper execution:

- signal is produced only from the current final bar;
- an order generated from that signal cannot fill on the same bar;
- fill happens against the next eligible bar under versioned deterministic execution assumptions;
- cash, position, equity, fills and SIGNAL/FILL chart markers remain reproducible;
- `GOOD` and `RECOVERED` quality are eligible; degraded/non-final bars fail closed.

## Live adapter

`qnext-data-collector` supervises the Python Strategy Lab paper runtime when the packaged `strategy-lab` module is present. The runtime listens on `127.0.0.1:18082` by default and reads finalized SYN+ history records from QNext file-backed market storage.

The live handoff is deliberately downstream of the existing SYN+ chart candle pipeline. No additional candle authority is introduced.

Environment overrides:

- `QNEXT_SYN_PLUS_PAPER_ADDR`
- `QNEXT_SYN_PLUS_PAPER_URL` for the Ops API proxy
- `QNEXT_PYTHON_BIN`
- `QNEXT_STRATEGY_LAB_ROOT`

## Persistence and revision semantics

Every accepted forward-paper bar is copied to an append-only session event log under:

`storage/paper/syn-plus/sessions/<session-key>/bars.jsonl`

Runtime configuration is stored at:

`storage/paper/syn-plus/runtime.json`

A process restart reconstructs the Q4 paper engine by replaying that as-observed event log. A later correction/revision of a candle whose open time has already been processed does not rewrite prior paper decisions. This preserves forward-test semantics while leaving canonical QNext market history free to apply its normal correction rules.

Turning Paper OFF freezes the current session. Turning it ON again starts a fresh forward session. Reset creates a fresh session while preserving the current ON/OFF state. Historical event logs remain file-backed for later attribution.

## Ops Console

The NIFTY-SYN+ card exposes:

- Paper ON / OFF;
- Reset Session;
- timeframe;
- initial capital;
- unit size;
- starter SMA-cross fast/slow parameters;
- position and average entry;
- mark, cash and equity;
- realized, unrealized and total P&L;
- maximum drawdown;
- closed-trade count and win rate;
- pending next-bar order state;
- SIGNAL/FILL marker count;
- current paper session and runtime errors.

Paper configuration is locked while the session is enabled. Controls are available only through the authenticated Ops API.

## Default configuration

- instrument: `QNEXT:NIFTY-SYN+`
- timeframe: `1m`
- initial capital: `100000`
- unit size: `1`
- starter strategy: SMA cross (`fast=2`, `slow=3`)

These defaults certify the pipeline; they are not a claim of trading efficacy.

## Remaining follow-on

1. fan SIGNAL/FILL markers into the Vela chart overlay rather than only exposing them through paper status;
2. tradable-leg paper mode using the Auto Leg Manager and real option/future quote execution simulation;
3. richer strategy selection from the Intelligence/optimization layer;
4. forward-paper attribution reports and comparison against deterministic backtests;
5. explicit promotion gates before any broker-shadow or live execution work.
