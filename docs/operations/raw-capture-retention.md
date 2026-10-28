# Raw market-feed capture retention

Status: staging-first implementation.

## Scope

QNext retains **30 UTC calendar days** of raw captured broker WebSocket frames by default: the current UTC date plus the preceding 29 dates. An hourly background sweep also runs at Market Core startup.

Only daily capture files matching `storage/raw/<provider>/YYYY/MM/YYYY-MM-DD.jsonl` are eligible, including optional `upstox-research` captures when enabled. Files must have an exact valid date matching their year/month folder, be regular files and have been unmodified for at least 24 hours.

**Outside scope:** `storage/market` canonical candles, SYN/SYN+ charts, `storage/futures-volume-ledger`, strategy-lab, research, replay, intelligence and other persistence. A cleanup pass never loads raw JSONL payloads or changes candle/volume calculations.

## Configuration

`QNEXT_RAW_RETENTION_DAYS=30` is the default in Market Core. `0` disables automatic raw cleanup. Invalid values also fail closed to disabled and log a warning. Values 1–3650 are supported. Configure this on the **Go Market Core runtime** (for the Cloudways supervisor: its sourced environment), not as an Admin PHP setting.

The process runs on startup, then each hour. Each pass has a 500ms duration ceiling, examines no more than 10,000 directory entries, and removes no more than 256 files. Later passes continue pending deletion. These are safety bounds; a pass marked incomplete means retention is catching up. Errors are logged without interrupting market ingestion.

The cleanup refuses a symlink storage root or raw root; skips symlinks in provider/year/month/day components, irregular files, mismatched dates and recently modified files. It deletes eligible files, not entire directories. No API endpoint allows user-supplied cleanup paths.

## Observation

Admin → Data Storage → Raw capture cleanup shows the setting, last run, number of files removed, bytes reclaimed, scan completeness and last error, via the authenticated existing feed-status proxy. The underlying Go feed-status snapshot includes `raw_retention`. No new publicly accessible admin endpoint is created.

## Validation / release gate

1. Run `go test ./internal/capture ./cmd/qnext-market-core`, `go test ./...`, and the existing Q0–Q6 CI gates. Confirm Ops Console typecheck and PHP API tests.
2. On an **isolated staging copy** of raw capture files, test: older file deletion, cutoff-day protection, today's active file protection, symlink safety, unknown filenames, non-raw directories untouched, invalid configuration, and repeated bounded sweeps.
3. Verify both ordinary Upstox `raw/upstox` and optional research `raw/upstox-research` are included.
4. Review disk use and raw retention status in Admin; verify Market Core remains available and candles, volume and WebSocket streaming remain unchanged.
5. Use normal DigiOps `digiops-release` packaging, with immutable artifact and separate staging install verification, before promoting to production.

A code rollback **does not undelete old raw files**. Back up any older raw captures needed for replay or regulatory/audit purposes before applying this cleanup policy. No retention policy substitutes for a compliant archival plan.
