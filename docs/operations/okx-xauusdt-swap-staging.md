# QNeXT — OKX XAU-USDT-SWAP staging feed

## Instrument and data authority
- QNeXT canonical ID: `OKX:XAU-USDT-SWAP` (ticker `XAU-USDT-SWAP`).
- OKX REST/WebSocket instrument ID: `XAU-USDT-SWAP`.
- Type: linear, USDT-settled perpetual derivative, **not** retail spot XAUUSD and **not** `OKX:XAUUSD-XPERP`.
- 2026-11-23 live exchange metadata: `state=live`, `instType=SWAP`, `ctVal=0.001`, `ctValCcy=XAU`, `ctType=linear`, `settleCcy=USDT`.
- Public `trades-all` is used for executed-trade ticks. Price in USDT and volume converted from contract units to XAU; no spot volume inference.
- Dedicated canonical history, 24/7 UTC calendar, 1m REST backfill, conditional contiguous 1s-to-5s recovery. Missing 5s intervals are not invented.
- XAUUSD X-Perp and NSE/BSE processing remain separate.

## Staging enable/disable
1. Deploy a Market Core build containing this integration and its matching Admin bundle.
2. Open **Admin → Market Data → OKX XAU-USDT-SWAP perpetual feed**.
3. Toggle **ON**, select **Save & restart Market Core**, confirm the restart and allow reconnect. Existing NSE/BSE feeds will briefly reconnect.
4. Check **configured=ON** separately from **LIVE**. LIVE requires recent actual executed-trade data.
5. Open QNeXT charts, select **Gold & Commodities · OKX → XAU-USDT-SWAP**, check 5s/15s/30s/1m candles and XAU volume.
6. Compare final 1m OHLCV with OKX exchange REST history and inspect 5s recovery; check weekends, UTC midnight, browser refresh and the separate USD X-Perp.
7. Verify pre-existing NIFTY-SYN+ volume and NSE/BSE feeds have not regressed.

## Control contract
- Admin API (authenticated session): `GET/PUT /qnext/admin/api/index.php?route=/okx-xauusdt`.
- JSON mutation: `{"enabled":true}` or `{"enabled":false}`; all other values are rejected.
- Private flag file: `config/okx-xauusdt.json`, independent from `config/okx-xauusd.json`.
- Legacy boot-time override only if no admin file: `QNEXT_OKX_XAUUSDT=1`. Default disabled; no automated switch to a different symbol.
- When the Market Core is too old, Admin must disable mutation and explain release requirements.
- Rollback: switch OFF via Admin and verify restart. Does not delete stored history.
- **GitHub CI success or exchange smoke test does not prove staging ingestion; chart, volume, outage/reconnect and hosting-region checks must be conducted after deployment.**

## Evidence
- Independent USDT swap live smoke: GitHub Actions workflow `OKX XAU-USDT-SWAP live preflight`.
- The USD X-Perp runbook remains in `docs/operations/okx-xauusd-release-gate.md`.
