# QNext engineering standards

## Architecture
- Keep market ingestion and normalization in Go Market Core. Provider-specific IDs stay in adapters; public selectors use immutable canonical IDs.
- New exchanges provide explicit instrument metadata and capability/market-session contracts. Do not enable a feed simply because an instrument is searchable.
- Share chart-independent catalog, selection and filtering logic; chart engine adapters own only engine-specific behavior. Keep existing SYN/SYN+ authority and history contracts intact.

## Performance
- No speculative feed subscriptions or per-result quote requests from symbol search. Use cached catalog metadata, lazy-load optional interfaces, and bound DOM result counts.
- Prefer shared transport and reusable modules to duplicated loops, request paths or dependencies. Measure before adding a heavyweight library.
- Treat source maps and test files as development artifacts. Report component bytes and checksums in every complete release; investigate growth rather than guessing at savings.
- Preserve canonical timeframes, price/volume provenance, and storage and subscription limits.

## Release safety
- The DigiOps release remains a full compatible runtime bundle until a verified incremental protocol exists. Use the release inventory to quantify changed and removed files. A delta must be bound to an approved exact base and target, reconstructed and verified before activation; never overlay a partial ZIP as though it were a complete release.
- Keep private runtime state, credentials, live history, and external ML dependencies outside release artifacts.
- CI must pass for the exact proposed commit. Browser smoke tests on staging are required for chart selection, timeframe restrictions, refresh, and live-feed changes. Do not claim deployment merely because an artifact exists.
