# Q6 Web Operations and Staging Deployment

Status: **CI CANDIDATE**

QNext staging operations are browser-managed after a one-time privileged bootstrap.

## Runtime boundary

```text
Browser
  |
  +-- /qnext/ ----------------------> Vela workspace
  |
  +-- /qnext/admin/ ----------------> static Ops Console
          |
          +-- /qnext/admin/api -----> private PHP Ops API
                                         |
                                         +-- health probes -> Go Market Core
                                         +-- atomic config/secrets
                                         +-- fixed helper actions
                                                  |
                                                  +-- systemd
                                                  +-- release activation
                                                  +-- rollback
```

PHP owns the conventional admin HTTP surface. Go Market Core remains the realtime market/candle/synthetic authority.

## Release layout

The single GitHub Actions artifact remains named **`digiops-release`**.

Its payload is:

```text
release-manifest.json
public/
  index.html
  assets/
  admin/
    index.html
    assets/
private/
  bin/
    qnext-market-core
  ops-api/
  config/
    q1-market.example.json
    q3-resilience.example.json
    q3-intelligence.example.json
  deploy/
    qnext-ops-helper
    qnext-ops-web
    qnext-market-core.service
    qnext-ops.env.example
    qnext-runtime.env.example
    qnext-sudoers.example
```

A server-side release is staged under:

```text
private_html/qnext/releases/<version>/
```

The active release is selected by:

```text
private_html/qnext/current -> releases/<version>
```

The previous good release is retained as:

```text
private_html/qnext/previous -> releases/<version>
```

Active mutable state remains outside releases:

```text
private_html/qnext/config/
private_html/qnext/secrets/
private_html/qnext/storage/
private_html/qnext/logs/
```

## Web operations

The first slice supports:

- runtime status
- Market Core `/health`, `/ready`, and `/version`
- start / stop / restart
- smoke test
- release inventory
- activate staged release
- rollback
- market configuration editor
- write-only Upstox/Dhan secret updates

There is intentionally no terminal, shell box, arbitrary process runner, arbitrary filesystem editor, or broker-secret read endpoint.

## Activation transaction

The root-owned helper validates the staged release, prepares the new public payload, switches the private `current` symlink, restarts Market Core, and executes health probes. If restart/probes fail, the previous symlink and public payload are restored before the helper exits with failure.

## One-time bootstrap

The host still needs one privileged bootstrap because a web process must not be allowed to grant itself service-manager permissions.

Bootstrap responsibilities:

1. create QNext private/public directories;
2. install `qnext-ops-helper` as `/usr/local/libexec/qnext-ops-helper`;
3. install `qnext-ops-web` as `/usr/local/bin/qnext-ops-web`;
4. install the narrow sudoers rules;
5. install the Market Core systemd unit;
6. configure PHP-FPM/web routing for `/qnext/admin/api` to `private_html/qnext/current/private/ops-api/public/index.php`;
7. set the PHP environment values including a staging admin token;
8. stage the first `digiops-release`.

After that, normal deploy/restart/configuration/rollback work is performed from the browser.

## Production hardening

Before public production:

- replace staging token auth with DigiOps authenticated admin sessions and role checks;
- add CSRF protection tied to that session;
- append every mutating operation to an immutable audit log;
- rate-limit authentication failures and mutating endpoints;
- integrate artifact download/staging directly with DigiOps so no manual artifact copy is needed.
