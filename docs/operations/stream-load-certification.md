# QNext Stream Load Certification

QNext Market Core is the single fan-out authority for browser charts. This certification protects that boundary from regressions as chart count grows.

## CI profile

The deterministic CI suite validates:

- 100 concurrent broker subscribers on one hot bar stream;
- 256 sequential bar updates delivered in order to every subscriber;
- slow-subscriber eviction without delaying a healthy subscriber;
- replay/resume availability after a slow subscriber is removed;
- explicit `resync_required` with reason `slow_consumer` rather than silent stream death;
- 50 real WebSocket clients subscribing through the HTTP upgrade handler;
- 32 ordered live updates delivered to every WebSocket client;
- p50, p95, and p99 in-process publish latency for 100 subscribers;
- a repeatable 100-subscriber broker benchmark in GitHub Actions.

The p99 correctness guard is intentionally broad at 50 ms. It is a regression tripwire for pathological locking or blocking, not a production network-latency SLA.

## Production interpretation

This CI suite certifies Market Core fan-out mechanics. It does not substitute for a staging soak across the public reverse proxy, TLS, browser runtime, Cloudways host, and real market traffic.

Production measurement should separately record:

```text
provider event time
  -> Market Core receive
  -> canonical tick accept
  -> candle/synthetic update
  -> broker publish
  -> public WebSocket frame
  -> browser receive
  -> chart render
```

The primary live dashboard should report p50/p95/p99 for tick-to-publish and, where browser telemetry is enabled, publish-to-render.
