# QNext Realtime Protocol Semantics

QNext uses one multiplexed browser WebSocket at `/api/v1/stream`.

## Operations

```text
HELLO
AUTH
SUBSCRIBE
SUBSCRIBED
SNAPSHOT
UPDATE
UNSUBSCRIBE
HEARTBEAT
RESUME
RESYNC_REQUIRED
ERROR
```

## Stream identity and sequence

Every logical subscription has a `stream_id`. While resumable state is retained, each published update has a monotonically increasing `seq`.

A reconnecting client may send a resume request containing the previous `stream_id` and last successfully applied sequence number.

If the server can replay the missing range, delivery resumes at `after_seq + 1`.

If that range is unavailable, the server emits `RESYNC_REQUIRED`. The client then obtains a fresh REST/history snapshot and establishes a new history/live boundary.

## History/live continuity

For bars, the canonical identity is:

```text
instrument_id + timeframe + open_time_ms
```

A client must never need to guess whether a bar from history is the same bar as a live update. Live updates may revise the forming bar and may deliver explicit historical corrections using a higher revision.

## Backpressure

The server must not allow a slow browser to accumulate an unbounded queue. Queue limits and disconnect thresholds are runtime configuration and must be observable and load-tested.

## Heartbeats

Heartbeat traffic is separate from market-data sequence semantics. Missing heartbeats may close a connection, but heartbeat messages do not create artificial market-data sequence gaps.
