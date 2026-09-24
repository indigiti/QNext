# Upstox V3 Wire Contract

QNext uses the Upstox V3 market-data flow behind the provider adapter boundary.

Official references:

- https://upstox.com/developer/api-documentation/get-market-data-feed-authorize-v3/
- https://upstox.com/developer/api-documentation/v3/get-market-data-feed/
- https://assets.upstox.com/feed/market-data-feed/v3/MarketDataFeed.proto

## Connection flow

1. QNext calls the V3 market-feed authorize endpoint with the server-side bearer token.
2. Upstox returns a one-time `wss://` URI.
3. QNext dials that URI.
4. QNext sends the subscription request encoded as JSON bytes in a binary WebSocket frame.
5. Upstox sends binary market-data frames.
6. A Protobuf decoder converts those frames into the provider envelope used by the QNext normalizer.
7. The normalizer resolves provider keys to canonical instruments and emits canonical ticks.

## Security boundary

The Upstox access token never reaches the browser. It is used only by the Go provider layer to obtain the one-time market-feed URI.

## Current slice

This slice implements and tests authorization, subscription encoding, connection orchestration, frame-decoder abstraction, and normalized tick delivery.

The concrete WebSocket transport and generated V3 Protobuf decoder are the next slice. They plug into `BinaryDialer` and `FeedDecoder` without changing the canonical pipeline.
