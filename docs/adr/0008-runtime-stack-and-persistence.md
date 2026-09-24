# ADR-0008: PHP web layer, no production Node server, no initial database

Status: Accepted

## Decision

QNext uses:

- PHP for the conventional web application/backend HTTP layer
- TypeScript + Vela in the browser
- Go for realtime market-data services
- Python for intelligence/research

There is no production Node.js application server.

Node.js may be used only as development/build tooling for TypeScript/Vela when required.

QNext also starts without an application database.

Durable market history, configuration, replay material, predictions, outcomes, and paper-trading state use versioned structured file persistence outside the public web root.

Redis, if introduced, is optional and ephemeral. It must not become authoritative historical storage.

## Rationale

This preserves the existing deployment model, keeps the critical market path inside Go, avoids introducing unnecessary long-running infrastructure, and allows database adoption later only if operational evidence justifies it.

## Consequences

- PHP must not proxy or process high-frequency market ticks.
- Browser realtime market data connects to the Go WebSocket service.
- File formats and directory layouts become versioned persistence contracts.
- File locking, atomic writes, compaction, retention, backup, and corruption recovery must be designed explicitly.
- A future database migration requires a separate ADR and migration plan; it is not assumed by the current architecture.
