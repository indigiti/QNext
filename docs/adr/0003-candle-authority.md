# ADR-0003: One canonical candle authority

Status: Accepted

## Decision

The Go Candle Engine is the only canonical candle authority.

Every downstream system—Vela, Python intelligence, replay, backtest, and paper trading—consumes the same canonical bars.

## Required semantics

A canonical bar records:

- instrument and timeframe
- OHLCV
- open/close time
- finality
- revision
- authority provider
- quality
- recovered/corrected flags
- source/candle-engine/synthetic versions where applicable

Independent reconstruction of supposedly identical candles in downstream services is prohibited.


## Authority clock

Canonical finality is owned by Market Core, not by arrival of the next provider tick. The candle engine uses the instrument's certified market calendar for live session admission and intraday alignment, and Market Core publishes/persists due final bars from its authority clock.
