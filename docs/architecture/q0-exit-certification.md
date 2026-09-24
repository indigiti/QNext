# Q0 Exit Certification

Status: PASS

Q0 establishes the QNext engineering foundation before runtime implementation begins.

## Contracts

- [x] QNext canonical naming
- [x] Runtime stack: PHP + TypeScript/Vela + Go + Python
- [x] No production Node.js server
- [x] No initial application database
- [x] Instrument / ProviderInstrument
- [x] CanonicalTick / Bar / AuthorityTransition
- [x] MarketStatus / ProviderHealth
- [x] SyntheticDefinition / SyntheticObservation
- [x] FeatureVector / Prediction / Outcome / ModelManifest
- [x] StrategySignal / Order / Fill / Position / Trade
- [x] BacktestRun / StrategyManifest
- [x] REST API baseline
- [x] realtime WebSocket baseline with resume/resync semantics

## Architecture decisions

- [x] Go realtime ownership
- [x] canonical symbol identity
- [x] single candle authority
- [x] Vela presentation boundary
- [x] deterministic provider authority
- [x] timestamp/finality/revision semantics
- [x] synthetic quality and lineage
- [x] PHP/no-Node/no-DB initial runtime model

## Tooling and certification

- [x] Protobuf package-aligned layout
- [x] Buf lint configuration
- [x] cross-language generation configuration for Go/Python/TypeScript
- [x] schema compatibility policy
- [x] deterministic NIFTY tick fixture
- [x] deterministic NIFTY-SYN five-strike parity fixture
- [x] automated fixture validator
- [x] GitHub Actions contract gate
- [x] CI PASS

## Q1 entry condition

Q1 may begin with the Go Market Core skeleton and deterministic canonical market pipeline. Vela integration and AI remain out of scope until the market-truth vertical slice is certified.
