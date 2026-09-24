# ADR-0001: Realtime ownership

- Status: Accepted
- Date: 2026-09-24

## Decision

The QNext Go Market Core is the sole authority for provider connectivity, market-event normalization, provider authority, synthetic calculations, canonical candle generation, recovery, integrity, subscriptions and realtime browser fan-out.

Vela owns presentation only. Python consumes canonical market data asynchronously. Strategy Lab consumes canonical data for deterministic replay, backtesting and simulated execution.

## Consequences

1. Browsers never receive broker/provider credentials.
2. Browsers never open direct market-data sessions with providers.
3. Python failure cannot interrupt chart data.
4. Canonical candles are generated once and reused by every downstream consumer.
5. Any future provider must adapt to QNext contracts instead of leaking provider-specific structures downstream.
