# ADR-0004: Vela is a presentation boundary

Status: Accepted

## Decision

Vela owns chart/workspace presentation. QNext integrates through a thin `QNextProvider` package.

The Vela layer must not contain:

- broker/provider credentials
- provider failover logic
- synthetic-market calculations
- canonical candle generation
- AI training
- paper-accounting logic

QNext custom indicators may render inside Vela, but their definitions remain QNext-owned and versioned.
