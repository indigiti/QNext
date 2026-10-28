# QNext 2026 cash/derivatives market session separation

Exchange source: NSE/FAOP/74467 (effective 2026-08-03); BSE notice 20260612-9.

| Market-data authority | Live trading window (IST) | Chart rule |
| --- | --- | --- |
| NSE/BSE cash index | 09:15–15:30 | Close at 15:30; never fabricate spot-index ticks after the cash session |
| NSE/BSE index futures/options | 09:15–15:40 | Accept live derivative trades until 15:40 |
| QNext SYN/SYN+ | Based on option-derived quotes | Continue valid price/volume candles until 15:40 |
| NIFTY/SENSEX futures proxy | Original futures source | Keep source watchdog/recovery active until 15:40; do not invent cash-index price candles |

Historical derivative candles before 2026-08-03 retain the original 15:30 session close.
The 15:40 boundary is half-open: a 15:39:59 event may be included, a
15:40:00 event must not open a new 5-second/15-second/1-minute candle.

The last complete 5-second child produces final subminute and higher-timeframe
bars at the appropriate market close. If source children are missing, retain
MISSING/UNKNOWN status; never manufacture missing OHLC or volume.

This release fixes segment calendar definitions, synthetic bucket boundaries,
clock finalization, 5s→15s/30s/1m→higher rollups, connection watchdog timing,
and futures-proxy silent-feed repair after the cash index stops ticking.

Staging certification must compare NIFTY/NIFTY-SYN/NIFTY-SYN+ and a BSE
market from 15:25 to 15:41 IST on an actual trading day, validate real
15:30–15:40 option/futures feed timestamps, and confirm no index-cash
price movement is introduced solely by derivatives ticks. CI success and
a DigiOps package alone do not establish this live gate.
