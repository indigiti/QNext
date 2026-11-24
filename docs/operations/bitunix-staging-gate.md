# Bitunix public crypto perpetual staging gate

The adapter uses the existing direct-feed registry, continuous UTC session,
canonical candle pipeline, storage, broker, backfill and readiness. No
credentials or trading endpoints are used. Existing OKX and Indian feeds are
unchanged by default. Initial supported pairs are BTCUSDT and ETHUSDT only;
all example entries are disabled. A metals session is not assumed continuous.

Official contracts reviewed 2026-11-24:
- https://www.bitunix.com/api-docs/futures/market/get_trading_pairs.html
- https://www.bitunix.com/api-docs/futures/websocket/public/Trade%20Channel.html
- https://www.bitunix.com/api-docs/futures/websocket/prepare/WebSocket.html
- https://www.bitunix.com/api-docs/futures/market/get_kline.html

The trade channel documents individual fill price/amount/time but no unique
trade ID or exchange sequence. Identical events are not deduplicated by price,
size and timestamp: those can be distinct executions. Local sequence is not
source sequence. Trade quality is conservatively DEGRADED, and the existing
carry-health gate prevents fabricated carry candles. A reconnect cannot prove
subminute completeness. One-minute LAST_PRICE historical bars use the
documented baseVol/quoteVol units only if turnover divided by volume falls
within the reported OHLC range (1% tolerance). The documentation example
appears to reverse these units; conflicting rows fail closed. No historical
5s API or public trade replay is assumed.

Enable one pair only in the existing private QNEXT_DIRECT_FEEDS_CONFIG, restart
the staging market core through DigiOps, then verify discovery, symbol catalog,
individual feed readiness, live timestamps, volume units versus independently
observed public market klines, 5s and 1m chart behaviour, reconnect, and
coexistence with OKX. Enable the second pair only after first-pair verification.
Measure CPU/RSS, publication gaps and persistence with the existing diagnostics.
Do not claim exact tick continuity or complete subminute backfill.

This is a staging candidate, not live certification. XAUUSDT/XAUUSD and Biquote
are not enabled. A live provider sample and review of any non-crypto calendar
are required before extending the allowed symbols.
