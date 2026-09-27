# NIFTY / NIFTY-SYN / NIFTY-SYN+ minimum-safe data path

## Goal

Reduce provider, CPU, memory, disk and browser load without changing the planned market logic or research coverage.

## Production lane

The production Upstox connection remains isolated from research and uses LTPC only.

- NIFTY spot: 1 instrument.
- NIFTY-SYN active basket: 5 strikes = 10 option legs.
- NIFTY-SYN warm basket: 7 strikes = 14 option legs total.
- Primary connection steady-state target: 15 instruments (NIFTY + 14 warm option legs).

The two extra warm strikes are retained deliberately. They allow the Auto Leg Manager to prepare a valid replacement generation before retiring old legs when ATM changes.

`QNEXT:NIFTY-SYN` remains the authoritative production synthetic. Its formula, candidate count, leg-age limits, time-skew limits and authority path are unchanged.

## Research lane

The second normal Upstox connection remains dedicated to Data Collector v1 and SYN+.

- NIFTY spot: 1 instrument.
- Current expiry ATM ±20: 41 strikes × CE/PE = 82 contracts.
- Next expiry ATM ±20: 41 strikes × CE/PE = 82 contracts.
- Research connection target: 165 instruments.

Tiering remains unchanged:

- current expiry ATM ±5: 22 contracts in `full` mode;
- remaining selected contracts: 142 in `option_greeks` mode.

No research contracts are removed by this optimization.

## Shared research-state hub

The research WebSocket frame is protobuf-decoded by `ResearchRunner` once. `ResearchStateHub` then converts each changed rich feed to `MarketState` once and fans the normalized update out to:

- 1-second option snapshots;
- 15-second option snapshots;
- `QNEXT:NIFTY-SYN+` shadow calculation.

Consumers retain their own bucket/candidate bookkeeping, but they no longer independently call `Feed.ResearchState()` for the same frame.

## Durable research writer

Option snapshots and SYN+ share one `JSONLStore` in the Data Collector process.

The store:

- serializes research writes;
- keeps active daily JSONL files open between snapshots;
- caps retained open file handles;
- fsyncs every append before returning;
- closes and syncs all files on shutdown.

This removes repeated open/close work without weakening the existing durability contract.

## Market activation

The shipped Q1 market example now sets:

```json
"active_markets": ["NIFTY"]
```

Other configured markets remain in the catalog and can be enabled later. During NIFTY/SYN/SYN+ validation they do not create provider subscriptions or synthetic managers.

## Browser fan-out

Browser subscriptions remain demand-driven by instrument + timeframe through the existing stream broker. Browser disconnects do not retire provider option legs; production and research calculation subscriptions stay warm independently of chart visibility.

## Footprint

Minimum-safe planned footprint while preserving production/research isolation:

- primary connection: 15 instruments;
- research connection: 165 instruments;
- total: 180 subscriptions across two isolated sockets.

The duplicated NIFTY spot and the production SYN option region are intentional isolation cost. Research failures must not become a dependency of the stable production synthetic.
