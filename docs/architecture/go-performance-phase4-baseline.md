# QNext Phase 4 — staging read-only baseline collector

## Why this exists

Release #92 (`d74b82f100f2`) added 4 segmented latency histograms. This
collector summarizes **actual** market-core telemetry into a small sanitized
report, with no WSS subscriptions, no state-changing requests and no pprof.
Collect during normal market traffic and compare repeatable windows before
proposing Phase 5 hot-path changes.

The collector must run **on the staging host**, or in a shell on that host
with access to its loopback HTTP API. It refuses remote URLs. A failed
connection from an external tool is **not** proof that QNext is down.

## Command

From a checkout of QNext on the staging machine:

```sh
python3 scripts/diagnostics/qnext_phase4_baseline.py \
  --base-url http://127.0.0.1:18080 \
  --expected-commit d74b82f100f2 \
  --samples 7 --interval 10 \
  --output /tmp/qnext-phase4-baseline.json
```

For per-process RSS and CPU (optional), append `--pid <market-core-PID>`.
The corresponding `market-core.pid` file is managed by the DigiOps runtime
under the private deployment root; verify its PID matches the market-core
process before running. The command does not enable profiler ports.

The script prints the sanitized report and optionally writes it to the path
provided by `--output`. It never includes raw broker stream details,
provider credentials, access tokens, full instrument identifiers or candle
prices. Treat operational reports as internal nonetheless.

## What the output means

- **Provider observations delta**: real tick activity seen by feedstatus; it
  must increase during live sampling if the provider is delivering ticks.
- **new_samples**: samples added in the window. Phase 4 samples one in 16
  operations. A p99 estimate needs at least roughly 100 *new* samples before
  it is useful for a window-level discussion.
- **latest_p50/p95/p99**: cumulative log2 histogram bounds since Go startup,
  **not interval percentiles**. Do not subtract percentiles.
- **tick_handle_latency**: includes canonical market processing, synthetic
  assembly, persistence queueing and publish; not upstream/provider delay.
- **publish_to_ws_write_latency**: live publish through successful server-side
  socket write; not browser display or network round trip.
- **slow_disconnects_delta** and **dropped_forming_delta**: deltas in potential
  fan-out/backpressure symptoms; forming drops are expected under coalescing
  and require correlating with subscribers and lag.
- **persistence.errors**, **capture.dropped/errors**: increments are warnings.
- **cpu_percent_one_core**, **rss_bytes**: only present with `--pid`. CPU is
  normalized to 100% of one CPU core, so multicore processes can exceed 100%.

Use at least two independent time windows with similar symbol/timeframe
subscriptions, market event rate, and feed status before evaluating gains.

## Next investigation

Use #92's ops dashboard alongside this report. If tick handler p99 dominates,
obtain a controlled loopback-only `pprof` CPU/mutex capture per
`docs/architecture/go-performance-phase4.md`. If socket write dominates,
check load/fanout/network. If browser dispatch dominates, profile Lightweight
and Vela chart callbacks independently. Never disable canonical candle
formation or history persistence as a performance shortcut.
