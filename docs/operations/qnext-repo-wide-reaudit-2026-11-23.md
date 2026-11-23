# QNext repository-wide code re-audit — 23 November 2026

**Audited ref:** `main` commit `89b3b00aa83a3d75197dd912883afa0247be7b53` (DigiOps release #139). This is a repository **risk-based code audit** across the version-controlled system, not an assertion that every statement of all 491 files has been manually verified. Repository tree, critical-path implementations, existing tests, open PRs/issues, CI jobs and release workflow were inspected. No authenticated staging host/Upstox broker session or signed production secrets were accessed. Statements about staging behavior must be treated as unverified until instrumented.

## Inventory, architecture and verification

- **491 tracked file blobs**: Market Core, Upstox/Dhan, deterministic synthetic price, canonical candles, futures volume proxy, JSONL history, WebSocket fan-out and 5s integrity; two chart renderers (Vela/PineTS and Lightweight Charts); PHP authenticated Ops API; Python Intelligence/Strategy Lab; schemas, deployment scripts and CI. The documented responsibility boundary remains: **Go owns live market truth; TypeScript/Vela presents; PHP administers; Python researches and simulates**.
- Latest `main` Q0 Contracts, Q1–Q5 Audit, QNext Release #139 are **GitHub-successful**. Go workflows run `go vet`, formatting, `go test -race ./...`, build, Python `unittest`, workspace `tsc`/Vitest/build and Vela parity. Release #139 passed its package download/identity verification. This certifies the **tested code and artifact shape**, not live index volume, 5-second continuity, Vela UI in a real browser, or installed staging SHA.
- No dedicated authenticated browser screenshot/performance/LCP/INP regression gate, 15-minute live REST-vs-WSS reconciliation gate or verified installed stage SHA was available in these sources. A recent green build does not prove any of those.
- Existing open PRs #123 (SYN+ volume reconciliation), #147 (Pine `request.security` timeframe compatibility), #105/#106 (Q7 readiness/history hardening), and open tracking issues #122, #111, #110 still require disposition against **current main**, rather than merging stale branches unreviewed. Current main already implements some of their fixes.

## Findings, ordered by risk

| Priority | Component | Source-backed finding | Status / action |
| --- | --- | --- | --- |
| **P1** | Market Core /ready | `httpapi/server.go` replies HTTP 200 unconditionally; async persistence backlog/errors and unconfigured ingestion cannot affect readiness. This can mask unready services in automated monitoring. | **Confirmed on main**. Tracked in #167; requires a separate, staging-certified readiness contract that preserves off-hours liveness. |
| **P1** | NSE/BSE calendar | `marketcalendar/registry.go` certifies the calendar through **2026-12-31** only (`SupportedTo: 2027-01-01`); later requests fail closed. Source does not contain verified 2027 holiday definitions. | **Confirmed approaching expiry**. Issue #166. Load official 2027 NSE/BSE notices; do not invent holidays/sessions. |
| **P1** | Public candle history | `httpapi/server.go` used to apply optional `limit<=5000` **after** `History.LoadRange`, and `history/range.go` scans every day in caller-provided range without a span cap. Public PHP GET proxy exposes this read path. Even short output can exhaust I/O/CPU. | **Confirmed on main; patched in this PR**: reject oversized timeframe-specific history ranges **before** storage reads (HTTP 413). Test each timeframe and preserve paged history access. |
| **P2** | Canonical history paths | `history/store.go:safeComponent` replaced slashes/colons but preserved exact `.` and `..` path segments, which `filepath.Join` interprets as navigation. An unexpected instrument/timeframe identity could escape the intended `storage/market` tree. This is a containment bug, not evidence of actual exploit or data access. | **Confirmed on main; patched in this PR** with regression cases and existing NSE/SYN directory naming preserved. |
| **P1** | SYN+ REST vs WSS volume | Existing issue #122 documents intermittent missing 1m futures-proxy volume; main has source-bar callbacks, direct proxy delta mirrors, revisioned corrections and focused Go tests. | **Mitigation present, live parity not certified**. Require session capture by exact symbol/timeframe/openTime through 15m/30m/1m with reconnect and sparse-price updates, including NIFTY and other enabled markets. |
| **P2** | Exchange close / missing buckets | `candle/engine.go`, `pipeline/subminute.go`, `pipeline/pipeline.go` now reject out-of-session tick formation, bound missing accounting, honor 15:40 derivative sessions after 2026-08-03 and include targeted tests. | **Code guards confirmed; older issue #111 not re-certified against live staging**. Compare 15:29:55→15:40 and next session without false missing/reconnect growth. |
| **P2** | Build dependency provenance | Both workspace packages pin top-level dependencies but CI uses `npm install --no-audit --no-fund`; no committed npm lockfile appears in the tracked tree for either package. Transitive resolutions can drift; CI lacks a dedicated dependency-vulnerability/SBOM gate. | **Reproducibility gap**. Adopt lockfile + `npm ci`, integrate risk-reviewed scans, and validate Vela/Pine against exact artifact. |
| **P2** | HTF Pine marker refresh | QNextProvider already normalizes bare minute/period history resolutions, and latest Vela v0.10.14 + PineTS bridge parity tests pass. This does not establish TradingView-equivalent live marker timing or every `request.security` revision behavior. | **Needs recorded browser/worker comparison** on finalized candles across 15s/1m/2m/5m and hard refresh. Open #147 may be stale; review changes against main before merge. |
| **P2** | NSE stock 1m | `NSEMinuteSync` fetches finalized Upstox V3 history sequentially, preserves equity volume, forces index OHLC price-only, and is gated behind `QNEXT_NSE_1M_SYNC=1`. Cap 12 symbols. | **Safe in code for enabled watchlist**, but live provider availability, symbol-key validity, API quota, historical completeness and in-chart selection need staged authenticated verification. Polling every 5m is not a true live feed. |
| **P2** | Ops/API production surface | PHP admin uses hashed token/session, restrictive cookie flags, per-request token/CSRF-style header for cookie writes, action validation, opt-in storage quarantine and explicit stopped-core check. `/api/v1/bars` and WS are intentionally public data paths; their quotas, proxy headers and reverse-proxy HTTPS assumptions remain deployment-specific. | No obvious immediate auth bypass established in reviewed paths; verify environment, reverse proxy headers, log redaction, abuse budgets, rate-limits and role constraints under staging. Do not infer a security certification from PHP lint. |
| **P3** | Admin presentation | Current Ops Console composes five generations of design CSS (`admin-shell.css`, `admin-sylva.css`, v2, v3, full screens). 13 screens were unit-tested as DOM contracts, not visually snapshotted. | **Accumulating CSS specificity/debt**; consolidate after actual staging screenshots/mobile keyboard checks, not by adding a sixth override. |
| **P3** | Documentation/CI coverage | Root README still describes an early Q6 baseline even though #139 exists; test coverage primarily unit/integration and package checks. | Refresh release/runbooks after a verified staging install; build a live acceptance artifact gate for version, 5s integrity, proxy volume, chart marker parity and operations. |

## Confirmed positive controls

- Canonical history rejects unfinalized bars and appends immutable revision records, reading the latest revision per bar. Raw-capture retention is separately scoped to `storage/raw`; it does not automatically prune canonical `storage/market` candles.
- Market Core source has a distinct futures-proxy volume attribution for SYN/SYN+; missing source volume defaults to `UNKNOWN` rather than fabricated traded volume. Live correction and same-bucket conditions require exchange-session verification.
- Historical 1m repair uses Upstox source bars; raw 5s cannot be recreated exactly from a 1m candle. Read-only integrity monitor and raw capture remain opt-in based on environment.
- WebSocket broker implements replay, backpressure/resync, same-origin browser checks and a token-authenticated internal SYN+ snapshot ingress. Security posture of proxy deployment/network interfaces not established.
- Python Intelligence has explicit candidate promotion gates and rollback pointer; Strategy Lab market order fills use next-bar opens and separate paper runtime instead of submitting live broker trades.
- Vela chart alert On/Off is a client preference filtering new script callbacks. It does **not** stop order/risk/server-side alert delivery or indicator calculations. No live test script was executed during this audit.

## Immediate changes in this PR

1. **Bound public history date spans before `LoadRange`**: up to 31 days for second-based, 400 days for minute-based, 1826 days for hour-based and 7305 days for day/week/month periods. Return HTTP 413 `history_range_too_large` + `max_days` for oversized requests. These bounds apply only to interactive browser GET, not historical repair.
2. **Keep canonical history paths beneath the market root**: sanitize exact `.`/`..` components while preserving canonical current instrument paths and filenames.
3. Add focused Go tests for each range class, `limit=5000` bypass attempts and file paths.

No changes to live ticks, SYN/SYN+ volume, session clock, Pine execution, service readiness, exchange holiday authority or the persisted JSONL format.

## Mandatory release/staging gate

- Validate Go format, race tests, Q0 schema checks, Q1–Q5, Vela/Pine parity, PHP Ops API smoke and DigiOps artifact verification.
- Verify allowed/oversized `/api/v1/bars` responses through actual public PHP proxy and chart history left-pan; check no unexpected HTTP 413 for normal chart fetches.
- Check installed staging SHA (`/version`), health vs readiness, history async write queue, service restart and role authentication. No claim that `/ready=200` proves sufficient durability.
- Capture market-session 15m NIFTY/SYN/SYN+ and BANKNIFTY if enabled: 5s completeness, 15s/30s/1m/5m REST vs WSS OHLC and volume, source/quality/revision/finality, before/after hard refresh, source gaps, replay, up/down synthetic signals.
- Validate after-market 15:30 cash vs 15:40 derivatives boundaries and calendar holiday windows with exact IST timestamps.
- Compare TradingView-equivalent Pine indicator signal markers/fills on the same finalized bars, including 1m chart / 2m or 5m higher timeframe and hard refresh.
- Verify no stage install was silently performed by simply building the artifact.

### Verification limits

This source audit does **not** include authenticated staging screenshots, raw broker tick data, live order operations, a penetration test, formal coverage percentages, p99 performance measurements or a certification of all 491 file bodies. Do not mark those as passed until real evidence has been collected and attached to an audit issue.
