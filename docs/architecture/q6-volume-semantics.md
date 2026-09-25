# QNext volume semantics

QNext carries `volume` on every bar, but the source depends on the instrument class.

## Tradable instruments

For equities, futures and options subscribed in Upstox LTPC mode, QNext reads `ltpc.ltq` (last-traded quantity) from each canonical tick and accumulates those quantities inside each QNext candle.

This keeps the live feed lightweight: no switch to Upstox `full` mode is required just to populate live candle volume.

## Cash indices

NIFTY, BANKNIFTY, MIDCPNIFTY, FINNIFTY, SENSEX and BANKEX are indices, not exchange-traded instruments. Upstox's LTPC index payload does not provide last-traded quantity. QNext therefore leaves live cash-index candle volume at zero instead of fabricating a value.

If a visual volume proxy is required for cash-index charts, that must be a separately defined signal (for example futures volume) and must not be labelled as native index volume.

## QNext synthetic indices

`*-SYN` prices are derived from active option parity legs. QNext uses the LTQ of each active option-leg tick that produces a synthetic update as a **basket-activity volume proxy**. Synthetic volume is therefore useful as an activity measure for the active parity basket, but it is not exchange volume for a traded synthetic instrument.

## Transport

REST `/bars` and the QNext WebSocket stream already carry the bar `volume` field. This change populates that field at the canonical tick/candle layer.
