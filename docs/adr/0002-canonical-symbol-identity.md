# ADR-0002: Provider-independent canonical symbol identity

Status: Accepted

## Decision

Logical instruments use provider-independent canonical IDs.

Example:

```text
instrument_id  NSE:NIFTY50
symbol         NIFTY
exchange       NSE
currency       INR
timezone       Asia/Kolkata
```

Provider mappings are metadata attached to that identity.

QNext public aliases may use forms such as `QNEXT:NIFTY` and `QNEXT:NIFTY-SYN`.

Provider-specific identifiers must not leak into strategy, intelligence, or Vela business logic.
