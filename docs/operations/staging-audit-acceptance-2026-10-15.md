# QNeXT staging audit acceptance — 2026-10-15

Scope: frontend, backend, security, visual modes, Vela/Lightweight charts, NIFTY/SYN/SYN+, sub-minute volume. This is **not** evidence of live staging validation. The branch remains a draft until the table is executed on the deployed version.

## Deployment identity and safe read-only API probe

1. Confirm deployed release manifest/commit equals the PR merge commit or an explicitly versioned release artifact. Do not compare against a prior audit index.
2. From a machine permitted to access stage.digiti.in, run:
   `python3 scripts/diagnostics/staging_audit_probe.py --url https://stage.digiti.in/qnext/`
3. Require 200 for both public web shells, public chart endpoints, correct JSON schemas, 403 for unauthenticated admin /config, /status, /diagnostics, and an empty public paper-marker snapshot.
4. Investigate any blocked HTTP 403/5xx, DNS, reverse-proxy or TLS issue; do not mark as passing.
5. Never transmit an admin token to the read-only probe, and do not use this script for write operations.

## Manual browser / visual matrix (not automated)

| ID | Check | Pass condition |
| --- | --- | --- |
| V01 | Workspace Dark/Light/System | Theme selector visible; all three choices work; selection persists on reload |
| V02 | Vela theme change | Vela chart/canvas and controls use selected theme; no chart/data corruption on reload |
| V03 | Lightweight theme change | Chart palette changes without disconnecting feed or replacing current symbol |
| V04 | Admin pre-auth theme | Sign-in gate remains readable with visible controls on both themes |
| V05 | Admin authenticated theme | Dashboard, sidebar, inputs, tables and badges remain readable across all themes |
| V06 | Mobile viewport | 320, 375, 720, 900, 1440px; no important controls clipped or inaccessible |
| V07 | Accessibility | Keyboard tab/focus, selector labels, contrast, reduced motion and sidebar behavior |
| V08 | Multi-chart | 1/2/4 chart modes; crosshair/time sync, symbol/timeframe selection, pane resize |
| V09 | Controls | Indicator list/search, layout, chart styles, admin navigation and logout respond correctly |
| V10 | Transport badge | WSS connected/REST fallback/connecting/idle report actual transport, not guaranteed tick freshness |

## Backend / data acceptance (stage)

| ID | Check | Pass condition |
| --- | --- | --- |
| D01 | NIFTY, SYN, SYN+ | Each symbol has correctly ordered OHLC, no duplicate timestamps, valid final/revision flags |
| D02 | 15s/30s/1m/3m/5m | Candle formation and enabled-display settings follow admin configuration |
| D03 | Volume | NIFTY and each synthetic symbol report legitimate volume source/provenance; unknown not rendered as zero |
| D04 | WebSocket | WSS connect, subscribe, update, unsubscribe, resume and reconnect behave correctly |
| D05 | REST fallback | Disconnect WSS and verify correct fallback marker, refresh behavior and resubscription |
| D06 | Pan-history | Left pan loads earlier bounded pages; no overlap duplicates, chart jumps or live candle loss |
| D07 | Gap repair | Trigger allowed authenticated repair, verify time-range integrity and idempotency |
| D08 | Data feed | Distinguish market closed, stale, provider outage and backfill; never claim false live health |
| D09 | Paper markers | Private by default; admin paper controls unaffected; public visibility only with explicit operator opt-in |
| D10 | Backend performance | Report version identity and p50/p95/p99 without publishing admin-only data |

## Admin security acceptance

- Fresh browser, invalid cookie, expired session and missing token: no authenticated admin UI or privileged responses.
- Correct token sign-in, sign-out, recovery-code creation/revoke/use, and rotation invalidate stale sessions.
- Cookie-based POST/PUT/DELETE without `X-QNext-Admin-Request: 1` returns 403. With the request header, authorized operations work. Header-token API clients are supported without browser CSRF header.
- Repeated incorrect sign-in/recovery returns 429 and Retry-After after configured limits. Verify actual `REMOTE_ADDR` under Cloudways; multiple users must not unexpectedly share the same throttle identity.
- Security headers, CORS and CSP checked on both public API and private admin endpoints.
- Service controls and promotions must be tested against a disposable staging service, not the production trading engine.

## Release gate

- [ ] GitHub CI green on exact PR head
- [ ] Staging release verified to match exact commit
- [ ] Read-only HTTP probe passed
- [ ] Visual matrix V01–V10 signed off
- [ ] Backend and data checks D01–D10 signed off, including an open-market session
- [ ] Auth tests signed off, including proxy behavior
- [ ] No regression to data ingestion, persistence, SYN+ paper trading or Intelligence Lab
- [ ] Recorded before/after metrics and rollback release identifier

No production promotion until all applicable checks pass. If access to staging is blocked, report 'not tested', never 'pass'. Rollback uses the existing versioned release pointer and must preserve file-backed history.
