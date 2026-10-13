# Split QNext core and Intelligence Lab ML releases

QNext core releases retain **both Go binaries**, Vela, the Admin console,
Strategy Lab and Intelligence Lab source code. The large, optional ML wheelhouse
is a **separate versioned GitHub Actions artifact**.

## Artifact layout

- `QNext Release` produces `digiops-release` (~8 MiB compressed in the audited build,
  excluding ML; actual size varies).
- `QNext ML Dependencies` produces
  `qnext-ml-dependencies-<SHA256 of wheelhouse.sha256>` when the dependency
  lock/packaging changes, on matching PRs, or by manual dispatch.
- The ML archive contains `requirements.lock`, `wheelhouse.sha256` and
  all transitive binary wheels needed for offline Python 3.11 installation.
- Core contains `private/config/intelligence-lab-ml.requirements.lock`, so the
  ML installer rejects a package that is incompatible with the active release.

## Install on the QNext host

1. Deploy/activate the new core artifact using the existing DigiOps procedure.
   Core Market Core, Data Collector, charts, Lab source and Ridge continue working.
2. Obtain the separate `qnext-ml-dependencies-...` GitHub Actions ZIP from the
   successful **QNext ML Dependencies** workflow. If no artifact exists after
   initial rollout, run the workflow manually once.
3. Unpack that ZIP into a **temporary private directory** (not the public webroot),
   so `requirements.lock` and `wheelhouse.sha256` are directly in that directory.
4. From an authorized shell account that owns `QNEXT_PRIVATE_ROOT`:

   ```bash
   export QNEXT_PRIVATE_ROOT=/home/ACCOUNT/private_html/qnext
   bash "$QNEXT_PRIVATE_ROOT/current/private/deploy/qnext-ml-dependencies" \
     install /path/to/unpacked-ml-wheelhouse
   bash "$QNEXT_PRIVATE_ROOT/current/private/deploy/qnext-ml-dependencies" status
   bash "$QNEXT_PRIVATE_ROOT/current/private/deploy/qnext-intelligence-lab-python" \
     bootstrap
   ```

   The `bootstrap` operation requires Python 3.11 and provisions the isolated
   Lab-only venv. Production Intelligence's Python runtime is never modified.
   Python 3.11 can be selected using `QNEXT_LAB_BOOTSTRAP_PYTHON`.

## Persistent storage and rollback

The installer validates all SHA-256 hashes and the exact release dependency
lock. It installs the verified archive immutably at:

```text
<private-root>/ml-dependencies/
  current -> versions/<wheelhouse-manifest-sha256>
  versions/
    <wheelhouse-manifest-sha256>/
      requirements.lock
      wheelhouse.sha256
      *.whl
```

Installation uses a host-local lock, stages files privately, and atomically
updates the `current` pointer. Older version directories remain available.
The isolated venv under `<private-root>/runtime/intelligence-lab-venv` is
reused if its manifest fingerprint matches, otherwise it is staged and swapped
after successful installation and import/version verification.

An ordinary QNext core update does not re-download the heavy wheelhouse.
Rollback of the core release works without altering the installed ML archive.
If a reverted core requires a different ML lock, install that matching ML
version; until then the Lab returns `UNAVAILABLE` and uses Ridge safely.
A missing wheelhouse also gives a Ridge fallback; it does **not** prevent
Market Core, Data Collector or the chart application from starting.

## Verification and CI

- `bash tests/certification/qnext-ml-dependencies.sh` checks installer
  verification, idempotency, corruption rejection, version switches and
  safe missing/mismatched-Lab status with synthetic wheels.
- `QNext Release` validates the core-only package and Ridge fallback.
- `QNext ML Dependencies` builds the real wheels, installs them into an
  isolated temporary root, bootstraps Python 3.11 and verifies the exact
  scikit-learn/LightGBM versions before publishing the independent package.

**Do not copy wheels into `public/` and do not discard old wheelhouse versions
until rollback requirements have been reviewed.** GitHub Actions artifacts have
finite retention; archive any approved dependency version needed for longer
rollback windows in your existing trusted private artifact store.
