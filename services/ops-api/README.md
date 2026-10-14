# QNext Ops API

The Ops API is the conventional PHP control plane behind the static QNext Operations Console.

It is deliberately outside the realtime market-data path.

## Security model

- same-origin browser API
- Admin token is exchanged once for an HttpOnly, SameSite=Strict browser session; ordinary Admin requests use the cookie rather than repeatedly sending the raw token
- first-time `/setup` is gated by a one-time `QNEXT_OPS_BOOTSTRAP_TOKEN` at the public bridge
- secrets are write-only
- no endpoint returns broker credentials
- no arbitrary shell command endpoint exists
- service/release actions call only the fixed `qnext-ops-web` wrapper
- release names are restricted to `[A-Za-z0-9._-]`
- config/secrets use atomic file replacement
- Market Core remains bound privately by default

Required PHP environment:

```text
QNEXT_PRIVATE_ROOT=/home/ACCOUNT/private_html/qnext
QNEXT_PUBLIC_ROOT=/home/ACCOUNT/public_html/qnext
QNEXT_MARKET_CORE_URL=http://127.0.0.1:18080
QNEXT_OPS_ADMIN_TOKEN=<optional legacy migration secret>
QNEXT_OPS_BOOTSTRAP_TOKEN=<one-time first-install secret>
QNEXT_OPS_HELPER=/usr/local/bin/qnext-ops-web
```

For a new installation, configure `QNEXT_OPS_BOOTSTRAP_TOKEN` with a strong random value before opening the Admin setup page. The first Admin token must match that bootstrap value; once Admin authentication is initialized, remove the bootstrap value from the PHP environment. Existing initialized deployments are unaffected.

The web wrapper invokes `sudo -n /usr/local/libexec/qnext-ops-helper`. The sudoers contract permits only the documented fixed operations.

The Admin LIVE BROKER section can also generate a backend performance report: authenticated
`POST /performance-report` records a single rate-limited, sanitized metric snapshot
every 10 seconds while an authenticated Admin page is open, and authenticated
`GET /performance-report` retrieves the most recently generated window.
After 60 seconds of sampling, the Admin button can download a JSON report.
The last 120 seconds (max 24 samples) are held under the private
`run/performance-baseline.json` path, with mode 0600. No access without
an Admin session or token, no public performance endpoint, no Go service
restart, no background process, no WebSocket subscription, and no profiler
activation. Opening Admin begins sampling; closing it stops new samples.
Percentiles are cumulative histograms, not end-to-end market-to-paint latencies.

The current file-backed Admin session is suitable for QNext stage/controlled production use. A future DigiOps identity integration can replace the local token bootstrap without changing the Market Core data path.
