# QNext admin design fidelity pass — Sylva screenshot reference

## Why another pass

PR #158 modernized navigation, and #159 refreshed the dashboard cards.
However, the reference screenshots reveal structural details that the prior
CSS did **not** implement: a floating, inset, fully rounded sidebar rather
than a full-height rail; a small multi-area menu; denser breadcrumb/search
placement; and a stable utility/profile footer. Simply recoloring the
original layout cannot reproduce those proportions.

The three screenshots shared by the requester (Sylva overview, pipeline,
team) are the reference. QNext is **not a CRM** and does not have Sylva's
three fictional company spaces. Instead, "AREAS" represents navigation
shortcuts to Market Data, Research, and Operations. No tenant switching,
symbol enabling or trading configuration is invoked by these shortcuts.

## Implementation

- Desktop floating sidebar, 12px inset, 24px radius, constrained viewport
  height, isolated scrolling menu (footer stays reachable at small heights).
- Functional navigation area shortcuts that map to QNext's existing views.
  All original admin routes stay available.
- Breadcrumb-like context in top bar (QNeXT / actual page title),
  inline magnifier search with working Ctrl/Cmd+K when narrow.
- Sidebar appearance preference (existing dark/light/system functionality)
  moved to footer. Generic "QNeXT Admin / Session protected" profile
  display; no fabricated person/account names or decorative avatars.
- More faithful lime/blue/coral shortcuts, quiet matte sidebar, heading and
  surface hierarchy. Existing telemetry cards and SYN+ quality data remain
  real and source-driven; no template revenue data is copied.
- Genuine mobile drawer scrim, click-outside dismissal, focus containment,
  and inert background so hidden panels cannot receive keyboard input while
  navigation is open. Dismiss on screen resizing; desktop dock unchanged.
- No added API calls, libraries, external font/asset requests, JSON schema
  changes, calculation changes, or chart code changes.

## Staging acceptance gate

The build/test run is necessary but **not visual certification**. After
deploying the candidate on staging, inspect authenticated Admin pages
at 1440, 1024, 768, 390 and 320px, both themes. Compare screenshot
layout geometry (sidebar inset, radius, navigation density, card rhythm,
search/footer) with the provided screenshots. Verify every admin route,
search/keyboard dock, responsive scroll, focus trapping and release controls.

Inspect with authenticated test data; do not expose admin tokens,
provider credentials, or trading records. In Developer Tools, confirm:
- No horizontal overflow or overlapped controls at each breakpoint.
- Live polling continues with independent backend REST/WSS semantics.
- No scroll jumps from telemetry refresh or route changes.
- Existing 5s/NIFTY SYN+ volume errors are not masked by visual styling.
- Core Web Vitals, memory, and network request counts are not degraded.
  No unmeasured "superfast" claim should be made.

If the currently installed staging artifact is older than the PR release,
the new layout will **not** be visible until DigiOps installs that release.
The GitHub build and staging deployment are different actions.

## Rollback

Previous DigiOps releases remain the rollback path. This is an admin-only
presentation change; it does not require market-core data migrations.
