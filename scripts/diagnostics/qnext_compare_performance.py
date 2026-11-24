"""Compare read-only staging reports, rejecting inadequate or mismatched workloads.

Cumulative server histograms are not browser chart paint latency.
An inconclusive comparison never certifies a deployment.
"""
import argparse
import json
import math
from pathlib import Path

from qnext_phase4_baseline import LATENCIES


def positive(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x) and x > 0


def compare(before, after, growth=0.15, workload_tolerance=0.20):
    issues, failures, metrics = [], [], {}
    for label, report in (("baseline", before), ("candidate", after)):
        if report.get("schema") != "qnext-phase4-staging-baseline-v1" or not report.get("expected_commit_matched"):
            issues.append(label + ": report identity not verified")
        if not positive(report.get("elapsed_seconds")) or not positive(report.get("provider_observations_delta")):
            issues.append(label + ": insufficient live activity")
        for name in LATENCIES:
            sample = report.get("latencies", {}).get(name, {})
            if not positive(sample.get("new_samples")) or sample["new_samples"] < 100:
                issues.append(label + ": insufficient new samples for " + name)
        for counter in ("persistence.errors", "capture.errors", "capture.dropped"):
            value = report.get("error_deltas", {}).get(counter)
            if not isinstance(value, (int, float)) or value < 0:
                issues.append(label + ": missing or reset " + counter)
            elif label == "candidate" and value > 0:
                failures.append(counter + ": increased during sampling")
        if not report.get("process"):
            issues.append(label + ": process CPU/RSS not measured")
        slow = report.get("counter_deltas", {}).get("slow_disconnects_delta")
        if not isinstance(slow, (int, float)) or slow < 0:
            issues.append(label + ": missing or reset slow disconnects")
        elif label == "candidate" and slow > 0:
            failures.append("slow disconnects increased")
    if not issues:
        rates = [r["provider_observations_delta"] / r["elapsed_seconds"] for r in (before, after)]
        if abs(rates[1] / rates[0] - 1) > workload_tolerance:
            issues.append("provider tick rates are not comparable")
        for name in ("subscribers", "active_streams", "websocket_connections"):
            values = [r.get("workload", {}).get(endpoint, {}).get(name)
                      for r in (before, after) for endpoint in ("first_broker", "last_broker")]
            if values[0] is None or any(v != values[0] for v in values):
                issues.append("workload differs or varies: " + name)
        if not positive(before["workload"]["first_broker"].get("subscribers")):
            issues.append("no live subscribers; socket latency not certifiable")
    if not issues:
        checks = [(key, before["process"].get(key), after["process"].get(key))
                  for key in ("cpu_percent_one_core", "rss_bytes_peak_sampled")]
        checks += [(name + ".cumulative_p95_us", before["latencies"][name].get("latest_p95_us"),
                    after["latencies"][name].get("latest_p95_us")) for name in LATENCIES]
        for name, old, new in checks:
            if not positive(old) or not positive(new):
                issues.append("missing/nonpositive measurement: " + name)
                continue
            change = new / old - 1
            metrics[name] = {"baseline": old, "candidate": new, "change_pct": round(100 * change, 2)}
            if change > growth:
                failures.append(name + ": growth exceeds reviewed tolerance")
    return {"status": "FAIL" if failures else "INCONCLUSIVE" if issues else "PASS",
            "failures": failures, "inconclusive": issues, "metrics": metrics,
            "scope": "server CPU/RSS and cumulative sampled latency only; browser paint and live continuity require separate checks"}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("baseline", type=Path)
    p.add_argument("candidate", type=Path)
    p.add_argument("--max-growth", type=float, default=0.15)
    args = p.parse_args()
    if not 0 <= args.max_growth <= 1:
        p.error("--max-growth must be 0..1")
    result = compare(json.loads(args.baseline.read_text()), json.loads(args.candidate.read_text()), args.max_growth)
    print(json.dumps(result, indent=2, sort_keys=True))
    return {"PASS": 0, "FAIL": 1, "INCONCLUSIVE": 2}[result["status"]]


if __name__ == "__main__":
    raise SystemExit(main())
