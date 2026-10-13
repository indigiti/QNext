# QNext Go Performance — Phase 1: streaming hot-path allocations

## Scope

This slice changes only the Go Market Core's in-process WebSocket broker.
Both Go executables, Vela, Intelligence Lab, file-backed history and
market-data normalization continue using their existing contracts.
In particular, WSS replay after browser resume (#107), final bar protection,
SYN+ volume updates and gap recovery must remain unchanged.

## Changes

- Cache each canonical stream ID once per stream rather than base64-encode
  it for every published forming/final bar and resync event.
- Grow the replay ring 16 → 32 → 64 → … up to its **unchanged retention limit**
  on demand rather than allocating the entire 1,024-event array at the first bar.
  A fully active stream still retains the exact same number of historical events.
- Reuse and compact subscribers' existing bounded queue storage instead of
  accumulating allocations as repeated dequeue operations consume slice capacity.

These changes target **allocation pressure and retained memory**, not data
semantics. They do not reduce the number of retained events, drop protected
final bars, alter sequence numbers, modify the WS JSON protocol or change
history fsync behavior.

## Reproduce the measurements

```bash
cd services/market-core
go test ./internal/stream -run '^$' -bench 'Benchmark(BrokerPublish|SubscriberQueue)' -benchmem -count=3
go test -race ./internal/stream
go test ./...
```

Compare the same benchmark on the previous main SHA and on this branch using
the same Go version and runner. Observe ns/op, B/op, allocs/op, peak RSS under
realistic subscriptions, and p95/p99 bar-publish latency separately. Synthetic
benchmarks cannot prove live Upstox-to-browser latency improvements.

## Next phases (not implemented here)

1. Record and compare before/after p50/p95/p99 **tick receipt to WSS publish**,
   plus broker subscribers, drops/coalesces and replay coverage.
2. Profile multi-symbol canonical 5s→15s/30s/1m and longer rollup allocation
   and global pipeline lock contention; do not weaken candle parity.
3. Profile JSONL history fsync latency, queue depth, flush lag and disk growth;
   retain durable finals and avoid deleting historical revisions.
4. Trace SYN+ live vs replay parity and research collector I/O before changing
   any aggregation and storage format.
