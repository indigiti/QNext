# Q1-Q5 Consolidated Audit

Status: **Q6 HANDOFF CANDIDATE**

This document identifies the authoritative implementation tracks that must be present together before Q6 begins.

## Authoritative phase baseline

- **Q1 — Market truth:** canonical Go market core, Upstox transport/normalization/recovery and exit certification.
- **Q2 — QNext workspace:** QNextProvider, Vela workspace, symbols, calendars, indicators and runtime/parity certification.
- **Q3 — Resilience:** Dhan hot standby, authority failover, gap recovery and resilience observability.
- **Q4 — Strategy Lab:** deterministic replay/backtesting, simulated execution and certification tests.
- **Q5 — Intelligence:** deterministic point-in-time features, predictions, outcomes and append-only persistence.

## Q3 audit repair

The original Q3 resilience branch contained the provider/router implementation and tests but was not safe to merge as-is: its runtime entry point referenced undefined resilience configuration variables and the HTTP resilience handler was not fully wired into the final Q2 HTTP options/routes.

The consolidation repair:

- preserves Q2 symbol/calendar/workspace wiring;
- loads resilience only when `QNEXT_RESILIENCE_CONFIG` is configured;
- requires `DHAN_CLIENT_ID` and `DHAN_ACCESS_TOKEN` only in resilience mode;
- registers Dhan mappings against the canonical symbol registry;
- keeps Upstox-only runtime behavior unchanged when resilience mode is disabled;
- exposes `GET /api/v1/resilience`;
- runs the Dhan/failover/gap/load tests as part of the full market-core race suite.

## Superseded tracks

Older Q4 and Q5 pull requests that target `q1-completion` are not authoritative after their later mainline replacements. The separate Q5 Vela/browser track is also not part of the Q5 baseline because the final Q2 workspace already owns the browser/Vela provider boundary.

## Q6 entry gate

Q6 may start only after the **Q1-Q5 Audit** workflow passes all four jobs:

1. Market Core (including Q1 + Q3 resilience tests, vet, race tests and build)
2. Q2 Workspace (typecheck, provider tests, Vela parity and production build)
3. Q3/Q5 Intelligence certification
4. Q4 Strategy Lab certification

The terminal gate emits `Q1_Q5_AUDIT_PASS`.
