# QNeXT visual refresh v2 — Screenshot-informed operations overview

## Reference
User-provided Sylva CRM screenshots from `/overview`, `/pipeline` and
`/team` informed the visual language. The live third-party demo could not be
retrieved through available web tooling, so no direct source/template code
was copied and the screenshots are the design basis.

The objective is not to recreate a CRM inside a trading platform. QNeXT has
existing Market Core, Ops API, market-data and Pine boundaries; this PR does
not change them.

## Changes
- Geometry and rhythm: stronger title scale and deliberate whitespace,
  dark/teal background, matte side navigation, legible bordered panels.
- Color: lime accent reserved for navigation selection, active actions and
  completeness indicators; restrained blue/olive/coral KPI cards.
- Typography: tuned system font stack (no external font loading),
  weight and tracking, tabular numerals, readable metadata.
- Dashboard rebuilt around actual QNeXT operational tasks: market Core,
  feed authority, current-day 5s integrity, installed release, configured
  symbol completeness, services, and direct navigation to specialist views.
- Visual bars correspond only to real per-symbol current-day health
  percentages exposed by the existing 5s health UI. Unknown percentages
  remain unknown; no fake revenue chart, performance trend, volume or
  fabricated market metrics.
- Consistent styling for indicators, storage, service controls and admin
  forms without a new JS framework or API.
- Static SVG navigation icons replace mismatched Unicode glyphs, with
  accessible labels preserved.
- Dashboard summary values now refresh every matching field via textContent
  rather than updating only the first occurrence. Focused rows are not
  replaced; view changes retain existing scroll behavior.
- Responsive adjustments for medium laptop, tablet and phone, reduced
  motion and higher-contrast preferences.

## Unchanged and protected
- Market Core and Upstox market feed / data subscription semantics
- Cash-index vs futures-volume source authority
- SYN/SYN+ candle repair, replay, ledger and session retention
- Trading workspace / Vela / Pine runtime
- Ops API authentication and all routes
- Deployment / rollback and access privileges

## Test / acceptance plan
1. Run Ops TypeScript, Vitest, Vite build and all relevant monorepo CI.
2. After staging deploy, compare screenshot layouts and accessibility at
   1440, 1024, 768, 390px, dark and light. Check keyboard dock and search.
3. Confirm that real-time API changes update both KPI and service-panel
   values without replacing focused inputs, losing scroll or showing fake
   health when data is missing.
4. Check text contrast, focus ring, status overflow, hidden/visible routes,
   admin forms and tables on all 13 views. Fix any UI regressions found.
5. Measure LCP/INP/CLS and JS heap for the admin console, not the trading
   workspace, before and after on the same device/network. No performance
   improvement is claimed without data.
6. Live full market-session integrity and WSS/Pine/volume parity are separate
   tests. A visual redesign cannot cure an upstream missing tick.

This PR is ready for code review after CI, but *not* production-certified
without the staging browser verification described above.
