# Phase G — Performance certification

## Source identity and scope

Baseline: `d1f9cab45affbc3bc959afd27eef2fdda6a7364c` (before B1).
Candidate: the checked-out Phase G revision, recorded in comparison.json.
Run on the same runner with the same Go toolchain, CPU setting and identical
benchmark bodies. Five observations per workload, 300 ms minimum per sample.
The benchmark gate compares medians of ns/op, B/op and allocs/op. A relative
regression above 15% (or a new allocation where baseline was zero) fails;
missing evidence or no improvement of at least 10% is inconclusive. These
are initial review thresholds, not universal latency SLAs. Do not relax a
threshold to pass a failing result without a documented review.

## CI evidence

- Q1–Q5 audit and Q0 contracts: full Go race/vet and fixture tests, workspace
  TypeScript/tests/Vela parity/build, intelligence and strategy tests.
- Phase G: B1 warm history range, B2 parallel disk scans, C1 daily rollup,
  D1 replay suffix and F1 SYN+ chain comparison, plus WSS 100–1000 clients.
- Phase G: Admin auth/session/throttle/storage and selective release tests.
- QNext Release: exact release inventory and compressed JS size budgets,
  artifact verification and immutable source/component identities.

Microbenchmark PASS is not live certification. This workflow does not
produce a DigiOps deployment or modify staging. Report raw results and
comparison.json separately from deploy artifacts.

## Staging acceptance (manual, not implied by CI)

Use the read-only collector in go-performance-phase4-baseline.md and
qnext_compare_performance.py. Record the exact component source commit
from the deployed manifest, correct market-core PID and comparable live
traffic windows (same subscribed instruments/timeframes, panes and process
age). Capture at least two windows per revision during active sessions.
If no pre-B1 live report exists, do not relabel a newer release as that
baseline; use a separately approved controlled baseline deployment or
state that live improvement against pre-B1 is unmeasured.

Check NSE/BSE and OKX, canonical 5s/15s/30s/1m, SYN/SYN+ volume and quality,
session-close finalization, history/live continuity and late revisions.
For both Vela and Lightweight, verify refresh/resume and panning, HTF
indicators before/after refresh, templates, alert toggles, 1/2/4-pane layout
where supported, and no duplicate 5s Admin display. Measure cold and warm
startup, transferred JS bytes, requests, long tasks and frame/paint timing
under the same browser, network and chart settings. Browser dispatch
histograms are not paint latency.

Record errors, missing/duplicate candles, persistence/capture loss,
slow disconnects, CPU, RSS, latency and browser observations. Require
functional parity, no unexplained regressions, passing exact artifact
verification and recorded evidence before marking Phase G complete.
Missing live evidence is INCONCLUSIVE, not PASS.
