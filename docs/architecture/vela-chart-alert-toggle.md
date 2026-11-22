# QNext Vela: chart script alert On / Off

## Scope

Add a simple **chart-level** script alert control to the QNext Vela
workspace, immediately alongside Vela's native Alerts bell.

- A native **bell switch** with a visible ON/OFF badge appears in the
  Vela topbar, in the same mobile overflow menu as Vela's other actions.
- ON is the default, preserving Vela's existing alert delivery.
- OFF suppresses **new Pine/indicator scripting alerts** before Vela
  receives them, so the built-in alert toast and bell notification
  history do not collect those new events.
- Existing alert notifications remain accessible in Vela's bell.
- Toggling takes effect on the *current running* Pine Worker/main-engine
  session without restarting it, re-fetching candles or modifying
  indicator source. No missed notifications are replayed on re-enable.
- The setting persists in the browser via
  `qnext-vela-chart-script-alerts-v1`, and synchronizes between tabs.
  Browser storage restriction results in a session-local preference.
- The setting is **workspace-wide**: all Vela charts/cells on this page
  share the same chart-notification On/Off state. It does not provide
  separate switches per individual chart in split layouts.

## Safety / boundaries

This is *not* a broker or server-side alert kill switch. It does **not**
disable risk controls, data collection, server notifications, order
management, trading signals or chart indicator computations. Buy/Sell
markers, fills, background colors, line plots and Pine runtime errors
remain unaffected.

Both the custom QNext indicator engine and Pine scripting engine are
wrapped at Vela's public `ScriptingEngine.execute` alert handler seam,
using a tiny guarded callback. The original engines and session handles
are otherwise delegated unchanged. Vela's native Alerts menu is not
overridden or monkey-patched.

The separate Lightweight Charts workspace does not host a Pine alert
engine and does not receive this Vela-specific control.

## Acceptance on staging

1. Verify ON by default and that a real Pine `alert()` appears in
   Vela's toast and bell list when triggered.
2. Select OFF and trigger the *same* alert on the *same running* Pine
   script. No new toast/bell event should be delivered.
3. Confirm Buy/Sell plotted shapes, indicator fills and 1m/5s candles
   still update during OFF, including the refresh/reconnect case.
4. Switch ON without reloading and trigger again; the new alert appears.
5. Reload in dark/light and test two tabs; saved state must persist.
6. Use the mobile overflow and keyboard to toggle.
7. Verify server-side risk alerts/orders and existing NIFTY SYN+
   futures-volume logic remain unchanged.

A passing build and unit test does not certify live market behavior.
Install via a DigiOps staging release and verify with a controlled
test script before production.
