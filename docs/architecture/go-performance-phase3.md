# QNext Go Streaming Performance — Phase 3: bounded replay memory and latency

## Scope and safety

This phase only changes the Go WebSocket broker lifecycle and ops observability.
It leaves the canonical candle pipeline, full-history persistence, both Go
executables, synthetic engine, Intelligence Lab, and data-collector authority
unchanged. Market data generation and recording **never depend on viewers**.

## Housekeeping

The market-core process runs a single broker housekeeping loop every **5 minutes**,
cancelled with its process context. Streams become eligible after **45 minutes
with no subscribers and no new publishes**.

- Published streams retain their stable ID, sequence number and newest bar.
  Replay backing storage is shrunk to one event, not deleted. The first
  subsequent publish expands storage again. This also preserves the latest
  broker bar lookup used by the SYN+ volume shadow bridge.
- Reconnect requests needing events older than the retained latest event
  receive `resync_required`, retaining the existing REST backfill path.
- Never-published, unsubscribed stream placeholders may be removed, since no
  sequence or candle data was ever generated.
- Active subscribers and recently published streams cannot be compacted.
- Operational counters expose cumulative compactions, reclaimed replay
  event slots, and removed never-published placeholders.
- No per-publish sweeps, timer per stream, or background goroutine per stream.

The `reclaimed_replay_slots` counter tracks allocations reclaimed, not peak
RSS or exact heap bytes. GC timing determines when heap memory is returned.

## Duration instrumentation

Sample one in **16** operations into fixed-size log2 microsecond histograms
with no per-sample heap allocation:

- `publish_latency`: time spent in `Broker.PublishBar`, including acquiring
  the broker lock and enqueueing to subscriber queues. This is **not**
  tick receipt-to-browser latency.
- `ws_write_latency`: time for server-side JSON serialization, waiting for
  the socket's write mutex, and WebSocket write. This is **not** client render
  or network round-trip latency.

`p50_us`, `p95_us`, and `p99_us` are histogram bucket **upper bounds**,
not exact percentile measurements. Each metric includes `samples`; treat
small counts (especially p99 with fewer than 100 samples) as provisional.
Metrics are cumulative since the process started; compare intervals or reset
via staging restart when profiling, while tracking event rate and workload.

## CI / release gate

```sh
cd services/market-core
gofmt -l .
go vet ./...
go test -race ./...
go test ./internal/stream -run '^$' -bench 'Benchmark(BrokerPublish|SubscriberQueue)' -benchmem -count=3
QNEXT_WSS_LOAD_CERT=1 go test ./internal/stream -run '^TestWebSocketLoadCertification$' -count=1 -v
```

In staging, check stream count, replay slots and cumulative reclaimed slots
45+ minutes after all clients disconnect. A resumed stream older than
retention must signal `resync_required` and recover with history. Measure
p95/p99 WSS write and broker publish latency under 3, 100 and 1000 client
loads, and separately instrument true provider-tick-to-chart-render latency
before deciding on candle/persistence refactors.
