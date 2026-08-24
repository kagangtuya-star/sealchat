# SealChat Model Agent Guide

This file defines model/persistence instructions for AI coding agents working under `model/`.

Repository-wide rules from the root `AGENTS.md` also apply.

## Scope

The `model/` package contains SealChat's persistence-facing data structures and database-related behavior.

The project uses GORM and supports multiple SQL backends, including:

- SQLite;
- MySQL;
- PostgreSQL.

Model code includes patterns such as:

- GORM model structs;
- explicit table names;
- GORM tags;
- indexes and unique indexes;
- timestamps and nullable fields;
- small persistence-oriented normalization methods;
- persistence helpers used by services.

Model changes are compatibility-sensitive because they affect existing databases, migrations, queries, imports, and service behavior.

## Role of the Model Layer

Keep persistence concerns here.

Appropriate responsibilities include:

- persistent struct definitions;
- GORM tags;
- table names;
- indexes;
- database representation;
- small persistence-specific helpers;
- normalization intrinsic to the stored model;
- reusable model-level query helpers where that is already the project pattern.

Do not put Fiber/HTTP concerns in model code.

Do not place large application workflows or user-facing permission orchestration into model hooks.

The service layer is the normal owner of multi-model business behavior.

## Existing Architecture

SealChat services may directly use `model.GetDB()` and GORM.

Do not introduce a mandatory repository/DAO abstraction across the project unless explicitly requested.

When adding a model helper, ensure it provides genuine persistence-level value rather than merely hiding one ordinary GORM call.

## Cross-Database Compatibility

Shared model/query behavior must work across SQLite, MySQL, and PostgreSQL unless explicitly backend-specific.

Be cautious with SQL features whose syntax or semantics differ, including:

- case-insensitive matching;
- JSON functions;
- boolean representations;
- datetime functions;
- string concatenation;
- upsert/conflict syntax;
- full-text search;
- collations;
- `RETURNING`;
- generated columns;
- partial indexes;
- identifier quoting.

Prefer portable GORM expressions and simple SQL where possible.

If backend-specific SQL is necessary, branch explicitly by dialect and test/document the behavior.

Do not fix one backend by silently breaking the others.

## Schema Changes

Do not change persistent schema for an implementation convenience.

Before adding/removing/renaming a field:

1. find existing migration/init behavior;
2. search all reads and writes;
3. inspect JSON/API exposure;
4. consider existing production rows;
5. consider zero/default/null values;
6. consider indexes and constraints;
7. consider downgrade/compatibility expectations if relevant.

A struct field change is not automatically a safe migration.

Do not rename a GORM column without an explicit migration/compatibility plan.

Do not delete a persisted field simply because current code appears not to read it.

## GORM Tags

Treat GORM tags as schema definitions.

Be deliberate with:

- `size`;
- `not null`;
- `default`;
- `index`;
- `uniqueIndex`;
- composite-index names;
- composite-index priority;
- column types.

When adding a query pattern that will be hot or large, consider whether an index is required.

Do not add indexes blindly. Each index increases write/storage cost.

Composite index order should follow actual filtering/sorting patterns.

## Base Models and IDs

Reuse existing base model types and ID conventions.

Do not introduce an incompatible primary-key strategy for one new model without a concrete requirement.

Keep ID lengths/types consistent with surrounding models and externally visible identifiers.

Do not assume an empty string is equivalent to a missing row in every query path unless existing model/service code intentionally uses that convention.

## NULL, Zero Values, and Defaults

Distinguish database `NULL` from Go zero values.

Use pointer/nullable fields when the difference matters.

Be especially careful with:

- `*time.Time`;
- optional strings;
- booleans with meaningful unset state;
- counters/version fields;
- foreign IDs.

Remember that GORM struct-based `Updates` may omit zero values, while map-based updates can explicitly write them.

Choose update form based on intended semantics.

Do not change `false`, `0`, or `""` persistence behavior accidentally.

## Normalize Methods

Small `Normalize()` methods are an established model pattern where normalization is intrinsic to the persisted entity.

Good uses include:

- trimming identifier/title fields;
- filling stable enum defaults;
- ensuring minimum version/default values.

Do not use model normalization to:

- perform network calls;
- query unrelated tables;
- enforce user permissions;
- rewrite user-authored rich content;
- trigger side effects.

Keep normalization deterministic.

If normalization must run before persistence, make sure callers actually invoke it or use the project's established hook pattern.

## Query Determinism

SQL row order is undefined unless explicitly ordered.

Any query whose behavior depends on order must use an explicit `ORDER BY`.

This is mandatory for:

- message timelines;
- pagination;
- recent records;
- priority/pinned content;
- deterministic exports;
- processing queues;
- cursor generation.

Do not rely on primary-key insertion order unless it is explicitly part of the query.

When two rows may have equal primary sort values, add a stable tie-breaker where pagination correctness requires it.

## Pagination

Pagination queries must be deterministic and gap/duplicate resistant.

For cursor/keyset pagination:

- ensure cursor fields match the query ordering;
- use stable tie-breakers;
- define inclusive/exclusive boundaries carefully;
- scope the cursor to the relevant resource/channel if required.

For offset pagination:

- understand that concurrent inserts/deletes can move rows;
- use it only where that behavior is acceptable.

Do not mix cursors generated from one filter/order with a different query.

## Updates and Concurrency

Avoid `Save` of an entire stale model when only a few fields should change.

Prefer targeted `Updates(map[string]any{...})` or equivalent when independent fields may be changed concurrently.

For compare-and-set behavior, include the expected old/version state in the `WHERE` clause and inspect `RowsAffected`.

Use database constraints for invariants that must survive concurrent processes.

A Go mutex cannot enforce a database invariant across multiple server processes.

## Transactions

Model helpers used inside a transaction should accept or use the transaction handle when necessary.

Do not secretly call the global DB from a helper that is expected to participate in the caller's transaction.

If adding a reusable query helper, consider accepting `*gorm.DB` when transactional composition is required.

Do not start nested/independent transactions without understanding GORM and backend semantics.

## Index and Query Review

When introducing or changing a frequently executed query, inspect:

- `WHERE` predicates;
- `ORDER BY`;
- join/preload behavior;
- expected cardinality;
- existing indexes.

For large message/channel/user tables, avoid unbounded scans.

Do not solve every query with a new index; prefer indexes that match real access paths.

For compound filters plus ordering, index column order matters.

## Relations and Preload

Use GORM relations/preloads deliberately.

Avoid accidental N+1 query patterns.

Do not preload large child collections when only counts or a few fields are required.

Use explicit joins/selects when that is clearer and consistent with nearby code.

Be cautious when adding cascading deletes or association behavior: it can alter existing data lifecycle semantics.

## Deletes

Understand whether a model uses:

- hard delete;
- soft delete;
- status flags;
- archival.

Do not replace one semantic with another casually.

For hard deletes, inspect dependent rows and external storage references.

For bulk delete/update, use narrow predicates and consider requiring an explicit scope guard.

Never write an unscoped bulk mutation without confirming it is intentional.

## Time

Use the project's established time conventions.

Prefer UTC for stored/server comparison when surrounding code does so.

Be explicit about:

- creation/update times;
- nullable event times;
- expiration;
- cursor timestamps;
- timezone conversion at API/UI boundaries.

Do not compare formatted time strings when native time/database comparison is available.

## Enums and Persistent Strings

String enums stored in the database are compatibility-sensitive.

Do not rename an enum value as cosmetic cleanup.

When adding a value:

- define its default/fallback behavior;
- update validators;
- update service/API handling;
- consider old clients/data.

Unknown stored values should fail or fall back according to the domain contract, not arbitrarily.

## Sensitive Fields

Persistent models may contain:

- credential hashes;
- token tails;
- external IDs;
- configuration;
- notification endpoints.

Do not add JSON exposure to sensitive fields without verifying that they may safely leave the server.

A database hash is still sensitive data even when not reversible.

Do not store plaintext secrets when the existing design uses hashed credentials.

## Hooks and Side Effects

Use GORM hooks sparingly.

Hooks should not hide substantial business workflows.

Avoid hooks that:

- broadcast WebSocket events;
- send notifications;
- call external APIs;
- mutate unrelated domain state;
- depend on request/user context.

These side effects are difficult to reason about and can execute unexpectedly during migrations/tests.

Prefer explicit service orchestration.

## Tests

For model changes, test persistence semantics when practical.

Important cases include:

- defaults;
- normalization;
- unique constraints;
- composite indexes/uniqueness;
- zero-value updates;
- nullable fields;
- deterministic ordering;
- pagination boundaries;
- migration compatibility.

When a query is intended to be cross-database, avoid tests that only prove SQLite-specific SQL unless the code is explicitly SQLite-only.

## Validation

Run focused model tests first:

```bash
go test ./model
```

Run affected service/API tests when model behavior is consumed there.

When practical:

```bash
go test ./...
```

For concurrency-sensitive persistence behavior, consider:

```bash
go test -race ./model ./service
```

The race detector does not replace database concurrency tests, but it can detect process-level data races.

## Diff Review

Before finishing a model change, explicitly check for:

- unintended column/table renames;
- changed defaults;
- changed JSON exposure;
- missing migration considerations;
- SQLite-specific SQL in shared code;
- missing `ORDER BY`;
- unsafe full-model saves;
- broad update/delete predicates;
- incorrect zero-value behavior;
- missing/incorrect index priority;
- sensitive fields becoming serializable;
- business side effects leaking into model hooks.

Persistence changes should be conservative and explicit.
