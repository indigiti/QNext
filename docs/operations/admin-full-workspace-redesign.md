# QNext Admin — complete specialist screen redesign

## Scope and design intent

The screenshot-informed sidebar and overview from Releases #132–#134
were not sufficient. The remaining 12 admin workspaces still rendered
the old generic cards and tiny status tiles. This change gives every
authenticated route a designed page, coherent component language, and
layout adapted to the underlying workflows.

All original content nodes, element IDs, event handlers, network requests,
permissions, source-of-truth metrics and Market Core services are retained.
The design is QNext-specific, not a copy of Sylva's CRM data or a replacement
for QNext's Go/PHP/Vela/Pine architecture.

## Pages covered

| Page | Redesigned content |
| --- | --- |
| Dashboard | Existing #159–160 overview retained |
| Market Data | Live provider, futures authority and enabled-market controls |
| Candles & History | Formation and chart timeframes displayed as paired cards, repair panel below |
| Synthetic | SYN+ snapshot, quality and paper evaluation in consistent panels |
| Indicators | Two-pane saved library and source editor with modern code surface |
| Intelligence Lab | Experiment cards, gates and operation controls |
| Quant Intelligence | Candidate cards, production metrics and lifecycle |
| Runtime | Service status, lifecycle controls and cron supervisor |
| Data Storage | Filesystem metrics, categories, raw retention and searchable symbol days |
| Diagnostics | Diagnostic metrics, log panel, copy/export controls |
| Releases | Constrained deployment/rollback panel with clear hierarchy |
| Settings | Workspace settings, broker secrets and wide market configuration editor |
| Security | Constrained protected access/recovery panel |

## Shared structural changes

- Every non-dashboard route has its own large, typo-scaled page heading,
  accurate operational scope, contextual links to other QNext areas, and
  a working section-index navigation generated from **actual** cards.
- New route frame remains a sibling of the original panel DOM. We do **not**
  remount widgets or duplicate polling. An existing top-level MutationObserver
  already detects late plugin panels, and the section index updates only
  when those panels change. Links scroll on **explicit click only**.
- Two legacy panels (Release and Broker Credentials) have their headings
  moved into a consistent card header. Existing form fields remain in the
  exact same nodes.
- Key-value/status cards, provider panels, commands, toggles, tables,
  indicator library, code editor, logs, and destructive buttons get a
  consistent surface, typography, focus state, spacing, and contrast.
- Dark/light theme, responsive columns, reduced motion, accessible navigation
  labels, keyboard focus on section jumps and narrow tables are covered.

## Architecture/performance boundaries

No changes to:
- Go Market Core and raw Upstox live feed, volume-proxy attribution, or
  feed recovery
- SYN/SYN+ or futures-volume ledger and historical candle corrections
- Market REST or WebSocket endpoints
- Vela chart, TradingView Pine compatibility, Python research workers
- Authentication, privileged Ops API, release/rollback permission checks
- Workspace account/settings backend or enabled-symbol persistence

Rendering adds 12 small static offscreen frames, of which 11 remain hidden
for any non-dashboard view. The section index is recalculated on route
changes and top-level card additions, **not** on every live quote/poll.
No JavaScript frameworks, network calls, font downloads or chart blur layers
are added.

## Mandatory staging acceptance

CI TypeScript/Vitest/Vite and repository checks establish code health.
After installing the candidate artifact in DigiOps staging, certify all
13 routes (including Dashboard) at 1440, 1024, 768, 390 and 320px in
dark and light. Inspect browser console and network activity while:

1. Navigating every route; section links must show only current-page cards.
2. Using keyboard navigation, Ctrl/Cmd+K search, compact dock, and mobile
   drawer; no inaccessible controls or horizontal overflow.
3. Editing a Pine source textarea and selecting saved indicators: no
   focus loss, editor reset or unsolicited scrolling on live telemetry.
4. Enabling/disabling index symbols and chart/candle intervals: controls
   must retain saved state and only their original explicit restart action
   may change the Market Core service.
5. Checking real NIFTY SYN+ source volume and Market Data panels during
   an actual trading session; "no data" must not be styled as "healthy."
6. Viewing release activation/rollback, admin access, broker credentials
   and storage Remove/Restore with role-appropriate access and safeguards.
7. Running Lighthouse and browser memory/network profiling against
   Release #134 and this build on equivalent staging data. Do not claim
   faster page loads without measuring.

Rollback to the prior DigiOps artifact; there are no storage migrations or
API schema changes.

## Explicit limits

This PR redesigns every **Ops Console admin route**. It does not redesign
the separate trading workspace/chart product, and it cannot certify
third-party Sylva screenshot-perfect parity without rendering staging in
a real authenticated browser. Those are separate deliverables.
