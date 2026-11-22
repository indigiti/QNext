# QNeXT admin shell redesign — Sylva-inspired phase 1

## Reference and scope
- Reference: https://demo.codeandchill.store/sylva/
- Public description: https://codeandchill.store/templates/sylva
- The Sylva demo could not be rendered using available remote inspection.
  Its publicly described interaction patterns (frosted-glass navigation,
  foldable dock, light/dark, keyboard search) informed this **original**
  QNeXT design. No copyrighted template source, screenshots, fonts or
  assets were copied.
- Phase 1 changes **only** the secured `apps/ops-console` presentation.
  No modifications to QNeXT Market Core, futures-volume authority, order
  handling, broker auth, Pine engine or production API routes.

## Changes
1. QNeXT-specific muted frosted-glass sidebar + header, opaque
   data-dense panels, stronger typography, consistent tokenized spacing,
   radius and dark/light contrast.
2. Desktop sidebar collapses to a compact icon dock with stable
   accessible labels, tooltip titles, and per-tab session preference.
   Sidebar buttons still use QNeXT's existing route state.
3. Keyboard: Ctrl/⌘+B toggles the desktop dock (except while editing a
   text field). Ctrl/⌘+K expands and focuses the existing route search;
   Enter opens the first matching page; Escape exits navigation.
4. Existing mobile drawer and logical navigation groups preserved.
   Focus-visible and reduced-motion rules are explicit.
5. The expensive dashboard subtree MutationObserver disconnects on
   non-dashboard views and reconnects only on Dashboard. Telemetry
   still updates in the source cards; the visible dashboard continues
   to display the current summaries without rerendering the shell.
6. No GSAP, React migration, fonts, third-party components, new API,
   background polling or additional broker connection.

## Multilayer architecture — unchanged
- **Admin UI:** Vite + TypeScript in `apps/ops-console`.
- **Ops API:** authenticated server-side operations boundary in
  `services/ops-api`, not a client-side database.
- **Market API:** REST/WebSocket interfaces to Go Market Core. Live
  prices, futures-volume authority and canonical candle storage remain
  independent of CSS, route transitions and panel visibility.
- **Workspace / indicators:** `apps/workspace` and PineWorkerEngine
  are not touched in this phase.

## Acceptance gates before staging merge / deployment
- `apps/ops-console`: TypeScript typecheck, Vitest contract suite, Vite
  build and generated deep-route materialization.
- Smoke every admin route in dark and light on desktop, tablet and
  phone. Verify authenticated-only loading; no unauthenticated API
  exposure; no frozen hidden views; dock navigation, keyboard shortcuts,
  tab order and focus restoration.
- Live feed, indicator editor, symbol settings and service controls must
  remain functional; switching pages must not initiate new Market Core
  subscriptions or change enabled symbols.
- Compare before/after navigation performance on identical staging
  data: route-change response, JS heap, main-thread time and console
  errors. Record a baseline rather than claiming unmeasured speedups.
- Confirm telemetry refreshes never reset scroll or remount focused
  inputs. Verify 5-second health row keyboard access.
- Test reduced-motion preferences; check Safari/WebKit blur fallback.
- Do not ship if any control is obscured by the mobile drawer or the
  collapsed dock, or if data tables lose contrast.

## Follow-on phases (not completed here)
- Per-page functional audit: admin, indicators, Market Core, settings,
  storage, release and rollback, with browser screenshots and API
  error-state checks.
- Performance profiling on real staging browser + slow device/network;
  targeted request deduplication only after identifying bottlenecks.
- Extend coherent design tokens to the user workspace and charting
  controls without altering TradingView/Pine parity, Vela or live data
  publication.
