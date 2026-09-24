#!/usr/bin/env python3
import json
import statistics
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

required = [
    "schemas/protobuf/qnext_market.proto",
    "schemas/protobuf/qnext_instrument.proto",
    "schemas/protobuf/qnext_synthetic.proto",
    "schemas/protobuf/qnext_intelligence.proto",
    "schemas/protobuf/qnext_strategy.proto",
    "schemas/openapi/qnext.yaml",
    "schemas/asyncapi/qnext-stream.yaml",
]
for relative in required:
    if not (ROOT / relative).is_file():
        raise SystemExit(f"missing required contract: {relative}")

ticks = []
with (ROOT / "fixtures/market/nifty_ticks.jsonl").open() as fh:
    for line in fh:
        if line.strip():
            ticks.append(json.loads(line))

if not ticks:
    raise SystemExit("NIFTY fixture is empty")

times = [x["event_time_ms"] for x in ticks]
seqs = [x["sequence"] for x in ticks]
if times != sorted(times):
    raise SystemExit("NIFTY fixture event times are not ordered")
if len(set(seqs)) != len(seqs):
    raise SystemExit("NIFTY fixture contains duplicate sequences")
if any(x["instrument_id"] != "NSE:NIFTY50" for x in ticks):
    raise SystemExit("NIFTY fixture contains unexpected instrument")

syn = json.loads((ROOT / "fixtures/synthetic/nifty_syn.json").read_text())
values = []
for c in syn["candidates"]:
    actual = c["strike"] + c["call"] - c["put"]
    if actual != c["expected"]:
        raise SystemExit(f"synthetic candidate mismatch at strike {c['strike']}")
    if c["accepted"]:
        values.append(actual)

if len(values) < syn["minimum_valid_candidates"]:
    raise SystemExit("not enough valid synthetic candidates")

actual_syn = statistics.median(values)
if actual_syn != syn["expected_synthetic"]:
    raise SystemExit(f"synthetic median mismatch: {actual_syn}")

print("Q0 fixture/contract validation: PASS")
