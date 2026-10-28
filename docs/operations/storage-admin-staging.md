# Storage Admin staging validation

QNeXT Admin now has a dedicated **Data Storage** page. It reports filesystem capacity and bounded directory metadata, and inventories canonical finalized candle JSONL files by the exact on-disk symbol key, UTC date and timeframe.

The inventory is **not** a verified market-data completeness, trading-session or volume-integrity audit. Filesystem capacity includes all data on the same filesystem, while scanned totals reflect only the inspected QNeXT directories. Partial scans are labeled partial.

## Safety defaults

- Storage inventory and health use the existing authenticated PHP Ops API only.
- Keep/Unkeep only changes a private, versioned retention manifest. No automatic cleanup job consumes it yet.
- **Remove/Restore are disabled by default**. A release or new Admin session never silently enables them.
- The PHP process must be explicitly configured with `QNEXT_STORAGE_ACTIONS_ENABLED=1` to allow Remove/Restore, and the handler checks the gate on every invocation. This setting belongs to the PHP Ops API environment, **not** the Go Market Core runtime.
- The backend additionally requires Market Core to be stopped, its health probe unavailable, and the tracked process PID absent. This does not replace operational confirmation on the server, especially if other services can write market history.
- Remove is quarantine, not deletion. Quarantine lives at `QNEXT_PRIVATE_ROOT/storage-quarantine/market`; it does **not** reclaim disk capacity. The action moves all stored candle timeframes for the chosen symbol/day. Keep protection, a 35-day minimum age and a 48-hour last-write guard apply.
- Quarantine does not move futures-volume-ledger, raw captures, strategy/research data, or off-host backups. They can therefore remain even if the candle files are moved. Charts/backtests may be incomplete during quarantine.

## Pre-merge gates

1. All head-SHA CI workflows pass, including Q0 Contracts, Q1–Q5 Audit and Q6 Web Ops.
2. The immutable staging candidate workflow builds the same `digiops-release` package from the staging branch, without deployment.
3. Review that `storage/` is outside the public web root and that PHP has only the required read permission by default; do not make live history world-writable.
4. Deploy the candidate via the established DigiOps staging route; verify Admin auth, pretty-route direct entry `/qnext/admin/storage/`, and unauthenticated API denial.
5. Inspect representative NIFTY, NIFTY-SYN and NIFTY-SYN+ symbol keys, timestamp basis, file counts, category totals and permissions against a **read-only** filesystem listing.
6. Confirm Remove/Restore endpoints return `409` while disabled, even if Market Core is stopped.

## Controlled mutation drill (separate gate)

**Only on an isolated staging copy**, first take and verify a snapshot/backup of the test history files. Prevent all Go writers, workers and repair jobs from writing to that copy; set the PHP-only flag there. Choose a fixture more than 35 days old and last modified more than 48 hours ago. Keep → Remove denied; Unkeep → Remove permitted; verify the active path has disappeared and quarantine is complete; Restore → verify byte-for-byte parity with the snapshot. Prove that recent dates, symlinks, name traversal, destination collisions and partially moved sets fail closed. Restore the feature gate to disabled after the drill. Never run this drill on the live market storage root.

## Rollback

Use the existing DigiOps release rollback for UI/API code. Data files are independent of deployments. Restore quarantined history before rolling back if any mutations were ever enabled; do not assume a code rollback restores filesystem data. Retention pins are stored under `config/storage-kept-days.json`, and private action audit records are under `run/storage-days-actions.jsonl`.

No MariaDB or Go candle/volume changes are needed.
