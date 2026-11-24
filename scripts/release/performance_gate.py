"""Enforce reviewed budgets on the exact release payload; no extra dependencies."""
import argparse
import gzip
import json
from pathlib import Path

from release_inventory import inventory


def check(root, budget):
    actual = inventory(root)
    failures = []
    expected = budget["components"]
    if set(actual["components"]) != set(expected):
        failures.append("Component set changed; review the budget explicitly")
    total = sum(row["bytes"] for row in actual["components"].values())
    if total > budget["max_total_bytes"]:
        failures.append(f"Total runtime bytes {total} > {budget['max_total_bytes']}")
    for name, row in actual["components"].items():
        if name not in expected:
            continue
        maximum = expected[name]["max_bytes"]
        if row["bytes"] > maximum:
            failures.append(f"{name}: {row['bytes']} > {maximum}")
    js = sorted((root / "public/assets").glob("*.js"))
    browser = [p for p in js if p.name.startswith("symbol-browser-")]
    if not js or len(browser) != 1:
        failures.append("Expected separate lazy symbol-browser JS chunk and workspace assets")
    compressed = sum(len(gzip.compress(p.read_bytes(), compresslevel=9, mtime=0)) for p in js)
    browser_bytes = sum(len(gzip.compress(p.read_bytes(), compresslevel=9, mtime=0)) for p in browser)
    for label, size, limit in [
        ("workspace total JS gzip", compressed, budget["workspace_js_gzip_max_bytes"]),
        ("symbol browser gzip", browser_bytes, budget["symbol_browser_js_gzip_max_bytes"]),
    ]:
        if size > limit:
            failures.append(f"{label}: {size} > {limit}")
    return {"total_runtime_bytes": total, "workspace_js_gzip_bytes": compressed,
            "symbol_browser_js_gzip_bytes": browser_bytes,
            "components": actual["components"], "failures": failures}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("release", type=Path)
    parser.add_argument("--budget", type=Path, default=Path(__file__).with_name("performance-budget.json"))
    args = parser.parse_args()
    budget = json.loads(args.budget.read_text())
    if budget["schema"] != "QNEXT.PERFORMANCE.BUDGET/1":
        raise SystemExit("Unsupported performance budget")
    result = check(args.release, budget)
    print(json.dumps(result, indent=2, sort_keys=True))
    return 1 if result["failures"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
