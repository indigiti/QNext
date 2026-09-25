# ADR-0009: File-backed first, MariaDB migration after live validation

Status: Accepted

## Decision

QNext will remain **file-backed with no application database** through initial production launch and live validation.

The file-backed contracts remain authoritative for:

- canonical market history;
- replay material;
- intelligence feature/prediction/outcome records;
- Strategy Lab and paper-trading artifacts;
- versioned operational configuration that is currently file-backed.

QNext must nevertheless keep persistence boundaries explicit so these records can later be projected or migrated into a relational store without changing canonical market, strategy, or intelligence domain contracts.

The planned first relational database target is **MariaDB**.

MariaDB adoption is **not automatic after a calendar month**. A migration review may begin only after QNext has completed at least **30 consecutive days of stable live operation** with the file-backed system.

## Minimum migration-readiness gate

Before MariaDB becomes authoritative for any data class, all of the following must be satisfied:

1. at least 30 consecutive days of live operation have completed;
2. the file-backed production baseline is considered stable enough to provide a trustworthy migration source;
3. observed query, concurrency, indexing, retention, reporting, tenancy, or operational needs justify adding a database;
4. backup and restore procedures for the existing file-backed records are verified;
5. the MariaDB schema and migration tooling preserve QNext identities, timestamps, revisions, hashes, lineage, quality and immutability rules;
6. migration is rehearsed against a copy of live data;
7. parity tests prove that file-backed and MariaDB-backed reads return equivalent canonical results for the certified sample periods;
8. rollback to the file-backed source remains possible until the database cutover is certified.

## Migration shape

The intended evolution is:

```text
Phase A — current
QNext services
      │
      ▼
versioned file-backed stores

Phase B — validation
versioned file-backed stores ──► MariaDB projection/shadow copy
                 │
                 └── remains authoritative

Phase C — certified cutover
QNext persistence interface
      ├── MariaDB authoritative query/transaction store
      └── immutable file/archive retention where useful
```

The migration should be performed per data class rather than as one all-at-once rewrite.

## Architecture requirements now

Even before MariaDB is introduced:

- persistence code should remain behind service/storage interfaces where practical;
- canonical IDs and versioned record schemas must not depend on filesystem path semantics;
- database-specific concepts must not enter Market Core domain contracts;
- append-only/revision semantics must remain reproducible after migration;
- deterministic replay must continue to work from immutable historical material;
- QNext must be able to compare file-backed and MariaDB-backed results during a shadow period.

## Rationale

The current file-backed approach keeps early operations simple and makes deterministic audit/replay straightforward while QNext is still proving its live market behavior.

Waiting for real production evidence avoids introducing database operations, schema migrations and recovery complexity before they provide measurable value.

MariaDB is selected as the planned relational target so QNext can later gain indexed querying, concurrency, transactional workflows, reporting and multi-user state without requiring a redesign of the market-truth architecture.

## Consequences

- No MariaDB dependency is introduced into Q1-Q5 closure work.
- Q6 production work must avoid coupling domain logic directly to JSONL/filesystem details.
- The first 30 live days are an observation and evidence-gathering period, not a countdown to mandatory migration.
- If the file-backed system remains sufficient after the review, migration may be deferred.
- If migration is justified, QNext will use shadow/parity validation before cutover.
