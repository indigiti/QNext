# QNeXT staging deployment handoff

**Status: explicit staging deployment handoff; never automatic production publication.**

QNext's `QNext Staging Candidate` workflow builds a tested release but **does not upload or install it on the Cloudways staging host**. Therefore a green GitHub Action is not a deploy-ready release in the Admin page.

## The two artifacts from the same successful workflow

- `digiops-release`: original public/private artifact for compatibility with the existing DigiOps direct deploy process. No behavior changes.
- `digiops-stage-package`: a new ZIP containing `qnext-staging-release.tar.gz` and `qnext-staging-release.tar.gz.sha256`. The inner POSIX tar preserves executable modes and relative symlinks that a GitHub Actions ZIP download may not. Use the tar for the **versioned server-side releases/** installation.

Both bundles contain the same commit's release payload, including public charts/admin, both Go binaries, ops-api, Strategy Lab, intelligence source, and the existing ML dependency lock (no bulky wheelhouse). **Only the staging tarball** adds small relative `qnext-market-core` and `qnext-data-collector` symlinks pointing to the uniquely versioned binaries, so systemd activation works without inflating the original DigiOps ZIP.

## Controlled stage-only installation (operator on staging server)

Download `digiops-stage-package` from the successful, reviewed GitHub Actions staging candidate workflow. On the staging host, under the authorized deployment user's private home directory:

```bash
set -euo pipefail
# Set explicitly for the STAGING app only, using its Cloudways account paths.
export QNEXT_PRIVATE_ROOT=/home/STAGING_ACCOUNT/private_html/qnext
export EXPECTED_QNEXT_COMMIT=FULL_40_CHARACTER_SHA_FROM_APPROVED_WORKFLOW

mkdir -p "$HOME/qnext-staging-package"
cd "$HOME/qnext-staging-package"
# Copy/download the authenticated Actions digiops-stage-package.zip here.
unzip -o digiops-stage-package.zip
sha256sum -c qnext-staging-release.tar.gz.sha256

tmpdir="$(mktemp -d)"
tar -xzf qnext-staging-release.tar.gz -C "$tmpdir"
bash "$tmpdir/private/deploy/qnext-stage-release" \
  "$tmpdir" "$QNEXT_PRIVATE_ROOT" "$EXPECTED_QNEXT_COMMIT"
rm -rf "$tmpdir"
```

The staging installer checks the exact SHA, release manifest and all required public/private paths. It rejects missing or absolute/suspicious symlinks, validates the Go binary pointers, refuses overwriting a staged version, and atomically installs to:

`$QNEXT_PRIVATE_ROOT/releases/<first-12-commit-digits>/`

It **does not** change `current` or `previous`, publish to the webroot, stop or restart any process, install ML dependencies, or touch file-backed market history.

## Deployment/activation paths are host-specific

1. **Cloudways direct mode:** Existing DigiOps direct deployment is authoritative. The Admin UI intentionally disables its `Activate selected release` / `Rollback previous` buttons. Staging a version into `releases/` alone will **not** deploy it. Pass the reviewed `digiops-release` to the existing direct DigiOps deployment process; do not assume systemd exists or replace it with an unapproved service manager.
2. **Privileged systemd managed mode:** The administrator must complete the documented one-time bootstrap. After verified staging, only an authorized admin may activate the exact version using the installed root-owned helper/Admin UI; the helper performs health checks and rollback. The new package includes stable relative symlinks compatible with `ExecStart=.../current/private/bin/qnext-market-core`.
3. **No host credentials/integration:** Delivery to the server, published release, and visual/live-feed validation remain **pending**. Do not label the GitHub artifact as deployed.

## Release gate after activation

- Check `/qnext/qnext-release.json` against `EXPECTED_QNEXT_COMMIT`.
- Run `python3 scripts/diagnostics/staging_audit_probe.py --url https://stage.digiti.in/qnext/ --expected-commit "$EXPECTED_QNEXT_COMMIT"` from a machine permitted to reach the host.
- Manually exercise admin authentication, chart themes, WSS/REST, NIFTY/SYN/SYN+, marker privacy, volume, and panning.
- Keep the previous staging release/rollback pointer and preserved file-backed storage.
- Production remains blocked until staging and rollback verification have completed.
