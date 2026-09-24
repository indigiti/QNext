# Upstox Reconnect and Recovery Boundary

QNext treats every provider disconnect as a possible market-data gap.

## Supervisor behavior

The Upstox supervisor:

1. runs the live stream;
2. records the latest canonical event time delivered to QNext;
3. applies bounded exponential reconnect backoff after a disconnect;
4. requests recovery for the interval between the last delivered event and the next reconnect attempt;
5. refuses to silently continue if the recovery hook reports failure;
6. reconnects only after the recovery step succeeds.

A sufficiently stable session resets reconnect backoff to its minimum.

## Recovery interface

The current Q1 slice defines the recovery contract but does not yet bind it to an Upstox historical REST endpoint.

The recovery request carries:

- provider identity;
- provider instrument keys;
- last canonical event time;
- recovery end time;
- disconnect cause.

The next history-recovery slice will implement that interface using provider historical/intraday APIs and canonical reconciliation rules.

## Canonical pipeline

Normalized provider ticks are forwarded through a small runtime adapter into the existing QNext canonical pipeline:

```text
Upstox WireClient
      ↓
Supervisor
      ↓
Normalized domain.Tick
      ↓
PipelineSink
      ↓
Candle Engine
      ↓
Append-only Canonical History
```

This preserves the QNext rule that provider reconnect logic never creates an independent candle path.
