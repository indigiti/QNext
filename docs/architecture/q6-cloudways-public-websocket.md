# QNext public WebSocket on Cloudways

## Target route

Public:

```text
wss://stage.digiti.in/qnext/api/v1/stream
```

Private Market Core:

```text
ws://127.0.0.1:18080/api/v1/stream
```

The release ships `public/.htaccess` with an application-level Apache rewrite/proxy rule for this exact path. REST market-data routes remain PHP bridges and are unchanged.

## Cloudways prerequisite

On Cloudways Flexible/Hybrid, `mod_proxy` and WebSocket proxy support must be enabled for the application/server. If the proxy module is unavailable, the WebSocket route will fail but the QNext workspace automatically remains live through the 250 ms REST forming-bar fallback.

Ask Cloudways support to enable the Apache proxy modules required for WebSocket reverse proxying if the Admin **Probe WSS** control reports `REST FALLBACK`.

## Verification

1. Deploy the current certified QNext release.
2. Open `/qnext/admin/`.
3. In **Market feed**, click **Probe WSS**.
4. Expected: `Browser transport: WSS READY`.
5. Open the workspace and confirm live candles continue moving.
6. Browser DevTools should show one persistent `wss://.../qnext/api/v1/stream` connection instead of continuous live `/bars/` polling.

The workspace retains REST fallback and stream resync support by design.
