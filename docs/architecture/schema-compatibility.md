# QNext Schema Compatibility Policy

QNext contracts are shared by Go, PHP, TypeScript, and Python consumers.

## Rules

1. Published Protobuf field numbers are never reused.
2. Removing or changing the meaning/type of a published field is a breaking change.
3. Additive optional fields are preferred.
4. Package/version namespaces change for intentionally incompatible schemas.
5. OpenAPI and AsyncAPI changes must preserve existing client behavior unless a versioned migration is approved.
6. Generated code is derivative output; source schemas in `schemas/` are authoritative.
7. Contract changes require CI linting plus compatibility review.
8. Persistence formats used by file-backed storage are also versioned contracts.

## Compatibility gate

Once Q0 is merged, subsequent changes to Protobuf contracts should run `buf breaking` against the `main` branch baseline before merge.
