# QNext Application Port Registry

**Repository:** `indigiti/QNext`  
**Last reviewed:** 2026-09-25  
**Owner:** QNext platform/runtime

This file is the canonical record of network ports used or reserved by QNext. Update it whenever a QNext component begins listening on a new port, changes its bind address, changes exposure, or retires a port.

## Production and staging ports

| Port | Protocol / direction | Bind or destination | Component | Purpose | Exposure | Status |
| ---: | --- | --- | --- | --- | --- | --- |
| 443/TCP | HTTPS inbound | Cloudways/web server | QNext public web surface | Serves `/qnext/`, `/qnext/admin/`, and public PHP/API bridge routes | Public | Required |
| 80/TCP | HTTP inbound | Cloudways/web server | Platform web server | Optional HTTP-to-HTTPS redirect handled by hosting platform | Public | Platform-managed |
| 18080/TCP | HTTP + WebSocket | `127.0.0.1:18080` | `qnext-market-core` | Internal QNext Market Core API, health/readiness, history, feed status, and WebSocket streaming | Loopback only | **Canonical QNext internal port** |
| 443/TCP | HTTPS/WSS outbound | Upstox / Dhan provider endpoints | `qnext-market-core` | Market-data, authorization, REST, and WebSocket provider traffic | Outbound only | Required when live feeds are enabled |

## Development-only ports

| Port | Protocol | Component | Purpose | Production use |
| ---: | --- | --- | --- | --- |
| 5173/TCP by default | HTTP | Vite development server | Local frontend development for Workspace / Ops Console. Vite may select another available port if 5173 is occupied. | None |

## Legacy / retired allocation

| Port | Previous use | Status | Action |
| ---: | --- | --- | --- |
| 8080/TCP | Earlier deployed `qnext-market-core` listener observed on `127.0.0.1:8080` | Legacy | Do not use for new QNext configuration. Migrate any remaining runtime override to `127.0.0.1:18080`. |

## Canonical runtime configuration

Production and staging Market Core should use:

```dotenv
QNEXT_HTTP_ADDR=127.0.0.1:18080
QNEXT_MARKET_CORE_URL=http://127.0.0.1:18080
```

Current repository references enforcing or consuming this allocation include:

- `deploy/qnext-runtime.env.example`
- `deploy/qnext-ops.env.example`
- `apps/workspace/public/api/v1/feed-status/index.php`
- `apps/workspace/vite.config.ts`
- `.github/workflows/qnext-release.yml`
- `services/market-core/cmd/qnext-market-core/main.go`

## Exposure policy

1. **Never expose port 18080 publicly.** It must remain bound to `127.0.0.1`.
2. Browser/public traffic enters through the hosting web server on HTTPS/443.
3. Public PHP/API bridge routes may call Market Core through `http://127.0.0.1:18080`.
4. Provider APIs and market feeds are outbound connections; no inbound provider port should be opened for them.
5. Development-server ports must not be required by a production release.
6. Before allocating a new QNext listener, check this registry and the host for conflicts.

## Change-control rule

Any pull request or deployment change that adds, removes, or changes a listening port, bind address, reverse-proxy target, WebSocket endpoint, or required outbound network port must update this file in the same change.

When retiring a port, move it to **Legacy / retired allocation** rather than deleting it immediately so deployment drift can be identified during server audits.
