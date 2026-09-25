# QNext Ops API

The Ops API is the conventional PHP control plane behind the static QNext Operations Console.

It is deliberately outside the realtime market-data path.

## Security model

- same-origin browser API
- first-time admin initialization is fail-closed and requires `X-QNext-Setup-Token` matching the server-provisioned `QNEXT_OPS_BOOTSTRAP_TOKEN`
- the bootstrap secret is not stored by the browser and is ignored permanently after the admin token is initialized
- initialized admin requests require the admin token in `X-QNext-Ops-Token`
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
QNEXT_OPS_BOOTSTRAP_TOKEN=<one-time random secret, minimum 16 characters>
QNEXT_OPS_HELPER=/usr/local/bin/qnext-ops-web
# Optional legacy adapter only:
# QNEXT_OPS_ADMIN_TOKEN=<pre-provisioned admin token>
```

The web wrapper invokes `sudo -n /usr/local/libexec/qnext-ops-helper`. The sudoers contract permits only the documented fixed operations.

For production, replace the staging token adapter with DigiOps authenticated session/role integration and CSRF protection.
