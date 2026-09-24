# Upstox V3 LTPC Decoder

QNext's first live-market slice requests Upstox V3 `ltpc` mode for NIFTY and the option legs used by NIFTY-SYN.

The official V3 schema defines:

- `FeedResponse.type` as field 1
- `FeedResponse.feeds` as field 2
- `FeedResponse.currentTs` as field 3
- `Feed.ltpc` as field 1
- LTPC fields for price, last-traded time, last-traded quantity and close price

QNext decodes only that subset using the Go Protobuf runtime's wire parser. Unknown fields are skipped, so richer Upstox payloads do not force the canonical market core to understand depth/Greeks before they are required.

Official schema:
https://assets.upstox.com/feed/market-data-feed/v3/MarketDataFeed.proto

## Why LTPC first

The initial certification target is canonical NIFTY plus NIFTY-SYN. Both require trustworthy prices and timestamps; they do not require full depth. Keeping the first provider mode minimal reduces subscription load and the amount of provider-specific state entering QNext.

Full/full_d30 support remains additive work behind the same provider boundary.
