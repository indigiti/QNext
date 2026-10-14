# QNext Performance Phase 4 — Instrumentation and opt-in CPU/lock profiling

## Goals and latency boundaries

Release #91 already measures broker publishing and the server's JSON/socket-write
call. Phase 4 adds **two server-side and one browser-local segment**. Each uses
its own process monotonic clock, so no cross-machine clock subtraction occurs.

| Metric | What it covers | What it does not cover |
| --- | --- | --- |
| `tick_handle_latency` | `MarketSink.Handle`: observation, volume proxy, canonical pipeline, synthetic assembly, history enqueue and broker publish | Upstox exchange/network/decode time |
| `publish_to_ws_write_latency` | Broker publish start to successful server WebSocket write return, live events only; includes delivery queue and write lock | Browser receive, rendering, TCP ACK |
| `publish_latency` | Broker mutex, replay, subscriber queue work | Candle generation |
| `ws_write_latency` | JSON serialization, write lock, WebSocket write | Client display |
| `window.__QNEXT_LATENCY__()` | Browser WebSocket message parse through synchronous QNext consumer callback completion (Vela/Lightweight) | Network time, next paint / GPU frame |

All duration samples are approximate log2-bucket upper bounds at 1/16
sampling; use sample counts and compare the same workload before/after.
Live publish-to-write samples exclude replay and resync control events. The
wire protocol stays `QNEXT.STREAM/1`; timing fields are internal only.

## Private CPU, mutex and blocking profiles

**Disabled by default.** No profiling route is exposed on the main market
HTTP server, admin dashboard or public proxy.

Set both of the following in a controlled staging-only process environment,
restart the market-core process and ensure no port forwarding/reverse proxy
maps `127.0.0.1:18081` to a public route:

```sh
QNEXT_LOCAL_PROFILING=1
QNEXT_LOCAL_PROFILING_TOKEN=<at-least-24-random-characters>
```

The profiler listens only on `127.0.0.1:18081` and requires the
`X-QNext-Profiling-Token` header on **every** request. Example on the
staging machine (keep secrets out of shell history/logs):

```sh
curl --fail -H "X-QNext-Profiling-Token: ${QNEXT_LOCAL_PROFILING_TOKEN}" \
  "http://127.0.0.1:18081/debug/pprof/profile?seconds=30" \
  -o /tmp/qnext-cpu.pprof
curl --fail -H "X-QNext-Profiling-Token: ${QNEXT_LOCAL_PROFILING_TOKEN}" \
  "http://127.0.0.1:18081/debug/pprof/mutex" \
  -o /tmp/qnext-mutex.pprof
go tool pprof -top /tmp/qnext-cpu.pprof
go tool pprof -top /tmp/qnext-mutex.pprof
```

The runtime enables sampled mutex and block profiling **only when explicitly
opted in**, and disables them at shutdown. CPU profiling itself can affect
latency; run a separate window to benchmark without pprof.

## Staging certification

1. Keep canonical 5s/15s/30s/1m and file-backed final candles running
   without chart subscribers.
2. Verify new fields are present in `feed-status` runtime broker statistics
   and ops LIVE BROKER after enough events.
3. With DevTools in workspace, run `window.__QNEXT_LATENCY__()` and collect
   browser-side dispatch samples on the same symbols/timeframes.
4. Compare 1, 100 and 1,000 consumers with normalized bar rates and
   `slow_disconnects`, p95/p99 and actual server RSS/CPU.
5. In the staging host's private profiler, identify concrete CPU and mutex
   contention before making any more aggressive candle/persistence changes.
6. Confirm profiling is unavailable when the two env settings are absent
   and no port is exposed on public routes.

Do not add server and browser percentile numbers to claim end-to-end latency:
the correct next step for absolute tick-to-paint latency requires synchronized
clocks, a sampled correlation ID and actual paint timing. This release makes
the current server and browser segments directly measurable instead.
