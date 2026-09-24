# Q5 — Vela / Browser Exit Certification

Status: **CANDIDATE**

Q5 is complete when the Q5 workflow is green and the browser layer remains presentation-only.

## Provider gates

- [x] thin `QNextProvider` package exists
- [x] REST history loads before live subscription
- [x] browser stream uses `QNEXT.STREAM/1`
- [x] canonical history/live bar identity is preserved
- [x] forming bars update without creating duplicates
- [x] finalized bars reject same-revision regressions
- [x] higher-revision historical corrections apply
- [x] sequence gaps trigger REST resynchronization
- [x] reconnect can issue `RESUME` with last applied sequence
- [x] `RESYNC_REQUIRED` establishes a fresh history/live boundary
- [x] no provider secrets or provider-specific failover exist in browser code

## Runtime gates

- [x] TypeScript is browser/build-time code
- [x] no production Node.js application server introduced
- [x] no application database introduced
- [x] Go remains canonical realtime/candle authority
- [x] Python Intelligence remains downstream-only
- [x] Strategy Lab remains separate from presentation

## Certification tests

- [x] forming/final bar reconciliation
- [x] higher revision correction
- [x] history-before-live subscription order
- [x] sequence-gap resync
- [x] cursor resume
- [x] Vela presentation binding
- [ ] final CI PASS

## Deferred

Full Vela package/theme import and workspace-shell composition can evolve independently around this certified provider boundary. Q5 deliberately avoids placing Vela internals into market-data ownership.
