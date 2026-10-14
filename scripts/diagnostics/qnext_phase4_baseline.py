#!/usr/bin/env python3
"""Read-only QNext #92 staging latency baseline; standard library only.

Run on the staging host against the loopback market-core API. The collector
never subscribes to streams, modifies runtime settings or exports raw payloads.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import time
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

LATENCIES = (
    "tick_handle_latency",
    "publish_latency",
    "publish_to_ws_write_latency",
    "ws_write_latency",
)
COUNTERS = ("slow_disconnects", "dropped_forming", "coalesced_forming")
IDENTITY = ("streams", "active_streams", "idle_streams", "subscribers",
            "websocket_connections", "replay_slots", "replay_bytes_estimated")


def require_loopback(url: str) -> str:
    parsed = urlsplit(url)
    if parsed.scheme != "http" or parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
        raise ValueError("Only local HTTP addresses are allowed; run on the staging host")
    if parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("URL must not contain credentials, query parameters or a fragment")
    return url.rstrip("/")


def request_json(base_url: str, path: str, timeout: float) -> dict:
    request = Request(base_url + path, headers={"Accept": "application/json"})
    with urlopen(request, timeout=timeout) as response:
        data = response.read(4 * 1024 * 1024 + 1)
        if len(data) > 4 * 1024 * 1024:
            raise ValueError("Telemetry response exceeds maximum size")
    document = json.loads(data)
    if not isinstance(document, dict):
        raise ValueError("Expected JSON object from " + path)
    return document


def number(source: dict, field: str):
    value = source.get(field)
    return value if isinstance(value, (int, float)) and not isinstance(value, bool) else None


def sum_provider_observations(providers) -> int:
    if not isinstance(providers, dict):
        return 0
    return sum(int(value.get("observed", 0)) for value in providers.values()
               if isinstance(value, dict) and number(value, "observed") is not None)


def sanitize_feed(feed: dict, at: float) -> dict:
    telemetry = feed.get("telemetry")
    if not isinstance(telemetry, dict):
        raise ValueError("Missing telemetry object")
    runtime = telemetry.get("runtime")
    if not isinstance(runtime, dict):
        raise ValueError("Missing runtime telemetry")
    broker = runtime.get("broker")
    if not isinstance(broker, dict):
        raise ValueError("Missing WebSocket broker telemetry")
    missing = [metric for metric in LATENCIES if not isinstance(broker.get(metric), dict)]
    if missing:
        raise ValueError("Phase 4 metrics missing: " + ", ".join(missing))
    history = runtime.get("history") or {}
    persistence = history.get("persistence") or {} if isinstance(history, dict) else {}
    capture = runtime.get("capture") or {}
    instruments = telemetry.get("instruments") or {}
    processed = [number(s, "last_processed_time_ms") for s in instruments.values()
                 if isinstance(s, dict)] if isinstance(instruments, dict) else []
    recent = max((v for v in processed if v and v > 0), default=None)
    return {
        "timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(at)),
        "broker": {field: number(broker, field) for field in (*IDENTITY, *COUNTERS)},
        "latencies": {metric: {field: number(broker[metric], field)
                    for field in ("samples", "p50_us", "p95_us", "p99_us")}
                    for metric in LATENCIES},
        "provider_observations": sum_provider_observations(telemetry.get("providers")),
        "newest_processed_age_ms": max(0, round(at * 1000 - recent)) if recent else None,
        "persistence": {field: number(persistence, field)
                        for field in ("queued", "pending", "errors", "flush_lag_ms")},
        "capture": {field: number(capture, field)
                    for field in ("dropped", "errors", "queued")},
    }


def process_stats(pid: int) -> dict:
    # /proc/<pid>/stat has a parenthesized command that may contain spaces.
    line = Path("/proc") .joinpath(str(pid), "stat").read_text()
    tail = line[line.rfind(")") + 2:].split()
    ticks = int(tail[11]) + int(tail[12])
    rss = int(tail[21]) * os.sysconf("SC_PAGE_SIZE")
    return {"cpu_ticks": ticks, "rss_bytes": rss}


def delta(first, last):
    if first is None or last is None:
        return None
    return last - first


def build_report(version: dict, samples: list[dict], expected_commit: str = "",
                 pid_data: list[dict] | None = None, elapsed_s: float | None = None) -> dict:
    if len(samples) < 2:
        raise ValueError("At least two samples are required")
    commit = str(version.get("commit", ""))
    if expected_commit and not commit.startswith(expected_commit):
        raise ValueError("Staging commit mismatch: expected " + expected_commit
                         + "; received " + commit)
    first, last = samples[0], samples[-1]
    warnings = []
    metrics = {}
    for name in LATENCIES:
        before = first["latencies"][name]
        after = last["latencies"][name]
        count = delta(before["samples"], after["samples"])
        metrics[name] = {"new_samples": count,
                         "latest_p50_us": after["p50_us"],
                         "latest_p95_us": after["p95_us"],
                         "latest_p99_us": after["p99_us"],
                         "total_samples_since_restart": after["samples"]}
        if count is None or count <= 0:
            warnings.append(name + ": no new sampled activity in collection window")
        elif count < 100:
            warnings.append(name + ": fewer than 100 NEW samples; p99 is provisional")
    counters = {field + "_delta": delta(first["broker"][field], last["broker"][field])
                for field in COUNTERS}
    for field in COUNTERS:
        if counters[field + "_delta"] is not None and counters[field + "_delta"] < 0:
            warnings.append(field + ": counter reset while collecting (possible restart)")
    for source in ("persistence", "capture"):
        for field in ("errors", "dropped"):
            if field not in first[source]:
                continue
            difference = delta(first[source][field], last[source][field])
            if difference is not None and difference > 0:
                warnings.append(source + "." + field + ": increased during collection")
    observations = delta(first["provider_observations"], last["provider_observations"])
    if observations is not None and observations <= 0:
        warnings.append("No new provider ticks observed; latency certification needs live traffic")
    report = {
        "schema": "qnext-phase4-staging-baseline-v1",
        "commit": commit,
        "version": version.get("version"),
        "started_at": version.get("started_at"),
        "expected_commit_matched": bool(expected_commit),
        "window": {"start": first["timestamp_utc"], "end": last["timestamp_utc"],
                   "samples": len(samples)},
        "latest_broker": last["broker"],
        "latencies": metrics,
        "provider_observations_delta": observations,
        "newest_processed_age_ms": last["newest_processed_age_ms"],
        "persistence": last["persistence"],
        "capture": last["capture"],
        "counter_deltas": counters,
        "warnings": warnings,
        "notes": [
            "Histogram percentiles are cumulative, sampled log2 upper bounds; not interval p99.",
            "No client render/paint, exchange-to-screen latency or end-to-end RTT measured.",
            "No WebSocket load or subscriptions created; CPU profiling is not enabled.",
        ],
    }
    if pid_data and len(pid_data) >= 2 and elapsed_s and elapsed_s > 0:
        ticks = delta(pid_data[0]["cpu_ticks"], pid_data[-1]["cpu_ticks"])
        report["process"] = {
            "cpu_percent_one_core": round(100 * ticks / os.sysconf("SC_CLK_TCK") / elapsed_s, 2)
                                    if ticks is not None and ticks >= 0 else None,
            "rss_bytes_last": pid_data[-1]["rss_bytes"],
            "rss_bytes_peak_sampled": max(s["rss_bytes"] for s in pid_data),
        }
    return report


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:18080")
    parser.add_argument("--expected-commit", default="d74b82f100f2")
    parser.add_argument("--samples", type=int, default=4)
    parser.add_argument("--interval", type=float, default=10.0)
    parser.add_argument("--timeout", type=float, default=5.0)
    parser.add_argument("--pid", type=int, default=0,
                        help="Optional market-core PID for Linux RSS and CPU measurement")
    parser.add_argument("--output", type=Path, help="Optional sanitized JSON report path")
    args = parser.parse_args(argv)
    if args.samples < 2 or args.samples > 60 or args.interval < 1 or args.interval > 300:
        parser.error("--samples must be 2..60 and --interval must be 1..300 seconds")
    if args.pid < 0:
        parser.error("--pid cannot be negative")
    try:
        base = require_loopback(args.base_url)
        version = request_json(base, "/version", args.timeout)
        start = time.monotonic()
        samples, pid_data = [], []
        for index in range(args.samples):
            if index:
                time.sleep(max(0.0, start + index * args.interval - time.monotonic()))
            samples.append(sanitize_feed(request_json(base, "/api/v1/feed-status", args.timeout),
                                         time.time()))
            if args.pid:
                pid_data.append(process_stats(args.pid))
        report = build_report(version, samples, args.expected_commit, pid_data,
                              time.monotonic() - start)
        output = json.dumps(report, indent=2, sort_keys=True)
        if args.output:
            args.output.write_text(output + "\n")
        print(output)
        return 0
    except (ValueError, OSError, TimeoutError, json.JSONDecodeError) as error:
        parser.exit(2, "QNext read-only baseline failed: " + str(error) + "\n")


if __name__ == "__main__":
    raise SystemExit(main())
