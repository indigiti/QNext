"""Compare the same five reviewed Go workloads on the same runner.

No live data, credentials, production profiling or deployment actions.
"""
from __future__ import annotations

import argparse
import json
import math
import re
import statistics
import subprocess
from pathlib import Path

BASELINE = "d1f9cab45affbc3bc959afd27eef2fdda6a7364c"
REPEATS = 5
MAX_GROWTH = 0.15
MIN_GAIN = 0.10
# Reuse the benchmark bodies already added by B1/B2/C1/D1/F1. Only
# the benchmark, not tests that depend on new internals, is copied to baseline.
WORKLOADS = {
    "BenchmarkCurrentDayRecentRange": ("history", "ordered_snapshot_test.go", ("testing", "time")),
    "BenchmarkHistoryParallelDiskReads": ("history", "disk_lock_test.go", ("fmt", "testing", "time")),
    "BenchmarkDailyRollupNoProxy": ("pipeline", "rollup_cpu_test.go", ("testing", "time")),
    "BenchmarkBrokerResumeRecentSuffix": ("stream", "broker_replay_suffix_test.go", ("testing", "time")),
    "BenchmarkSynPlusEvaluateChain": ("research", "research_perf_test.go", ("fmt", "testing", "time")),
}
METRICS = ("ns/op", "B/op", "allocs/op")
LINE = re.compile(r"^(Benchmark\w+)-\d+\s+\d+\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op\s*$")


def parse(text):
    rows = {}
    for line in text.splitlines():
        match = LINE.fullmatch(line)
        if match and match[1] in WORKLOADS:
            rows.setdefault(match[1], []).append(dict(zip(METRICS, map(float, match.groups()[1:]))))
    return rows


def compare(before, after):
    results, issues, failures, gains = {}, [], [], []
    for name in WORKLOADS:
        left, right = before.get(name, []), after.get(name, [])
        if len(left) != REPEATS or len(right) != REPEATS:
            issues.append(f"{name}: expected {REPEATS} samples on each revision")
            continue
        results[name] = {}
        for metric in METRICS:
            values = [row.get(metric) for row in left + right]
            if any(not isinstance(v, (float, int)) or isinstance(v, bool) or not math.isfinite(v)
                   or v < 0 or (metric == "ns/op" and v == 0) for v in values):
                issues.append(f"{name}: invalid {metric} measurements")
                continue
            old = statistics.median(values[:REPEATS])
            new = statistics.median(values[REPEATS:])
            change = (new / old - 1) * 100 if old else (0.0 if new == 0 else None)
            label = f"{name} {metric}"
            results[name][metric] = {"baseline": old, "candidate": new, "change_pct": round(change, 2) if change is not None else None}
            if (old == 0 and new > 0) or (old > 0 and new > old * (1 + MAX_GROWTH)):
                failures.append(label + ": regression exceeds 15%")
            if old > 0 and new <= old * (1 - MIN_GAIN):
                gains.append(label)
    if not issues and not failures and not gains:
        issues.append("no measured improvement of at least 10%")
    return {
        "schema": "QNEXT.PERFORMANCE.MICROBENCH/1",
        "status": "FAIL" if failures else "INCONCLUSIVE" if issues else "PASS",
        "failures": failures, "inconclusive": issues, "measured_gains": gains,
        "metrics": results, "samples_per_revision": REPEATS,
        "scope": "Go microbenchmarks only; not live staging, browser paint or whole-system certification",
    }


def run(cmd, cwd):
    result = subprocess.run(cmd, cwd=cwd, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, check=False)
    print(result.stdout, end="", flush=True)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {cmd}")
    return result.stdout


def install_baseline_benchmarks(baseline, candidate):
    for name, (package, file, imports) in WORKLOADS.items():
        relative = Path("services/market-core/internal") / package
        source = (candidate / relative / file).read_text()
        marker = f"func {name}(b *testing.B) {{"
        if source.count(marker) != 1:
            raise ValueError(f"missing or ambiguous workload: {name}")
        body = marker + source.split(marker, 1)[1]
        # The reviewed benchmark is the final function of its source file.
        if re.search(r"^func ", body[len(marker):], re.MULTILINE):
            raise ValueError(f"benchmark must remain the final function: {name}")
        header = f"package {package}\n\nimport (\n" + "".join(f'\t"{item}"\n' for item in imports) + ")\n\n"
        (baseline / relative / f"phase_g_{name.lower()}_test.go").write_text(header + body)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    baseline, candidate, output = (p.resolve() for p in (args.baseline, args.candidate, args.output))
    if baseline == candidate:
        parser.error("baseline and candidate must be separate checkouts")
    output.mkdir(parents=True, exist_ok=True)
    identities = {label: run(["git", "rev-parse", "HEAD"], root).strip()
                  for label, root in (("baseline", baseline), ("candidate", candidate))}
    if identities["baseline"] != BASELINE or identities["candidate"] == BASELINE:
        parser.error("unexpected baseline or candidate identity")
    if run(["git", "status", "--porcelain"], baseline).strip() or run(["git", "status", "--porcelain"], candidate).strip():
        parser.error("both checkouts must be clean")
    install_baseline_benchmarks(baseline, candidate)
    go_version = run(["go", "version"], candidate).strip()
    for root in (baseline, candidate):
        run(["go", "mod", "download"], root / "services/market-core")
    logs = {"baseline": [], "candidate": []}
    # Alternate which revision runs first by workload to limit ordering bias.
    for i, (name, (package, _, _)) in enumerate(WORKLOADS.items()):
        order = [("baseline", baseline), ("candidate", candidate)]
        if i % 2:
            order.reverse()
        for label, root in order:
            log = run(["go", "test", "./internal/" + package, "-run", "^$", "-bench", "^" + name + "$",
                       "-benchmem", "-count", str(REPEATS), "-cpu", "4", "-benchtime", "300ms", "-timeout", "5m"],
                      root / "services/market-core")
            logs[label].append(log)
            (output / (label + ".txt")).write_text("\n".join(logs[label]))
    result = compare(*(parse("\n".join(logs[label])) for label in ("baseline", "candidate")))
    result.update({"commits": identities, "go_version": go_version, "cpu": 4,
                   "max_growth_pct": MAX_GROWTH * 100, "min_gain_pct": MIN_GAIN * 100})
    report = json.dumps(result, indent=2, sort_keys=True)
    (output / "comparison.json").write_text(report + "\n")
    print(report)
    return {"PASS": 0, "FAIL": 1, "INCONCLUSIVE": 2}[result["status"]]


if __name__ == "__main__":
    raise SystemExit(main())
