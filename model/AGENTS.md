# SealChat Model / Database Agent Guide

This file defines persistence-layer rules for AI coding agents working under `model/`.

Repository-wide rules from the root `AGENTS.md` also apply. Keep changes small, compatibility-safe, and limited to the requested scope.

## Scope

`model/` owns persistence-facing behavior:

- GORM models and table definitions;
- fields, tags, indexes, constraints, timestamps, and nullable values;
- reusable persistence/query helpers;
- persistence-specific normalization;
- database behavior shared by services.

SealChat supports:

- SQLite;
- MySQL;
- PostgreSQL.

Do not put Fiber/HTTP behavior, permission orchestration, WebSocket broadcasting, external API calls, or large multi-model workflows in `model/`. Those normally belong in `service/` or `api/`.

Do not introduce a mandatory repository/DAO layer. Existing services may use `model.GetDB()` and GORM directly.

## Compatibility

Treat schema, GORM tags, stored values, enum strings, defaults, indexes, and query semantics as compatibility-sensitive.

Before changing persistent structure or behavior:

1. inspect existing migrations/init behavior;
2. search all readers and writers;
3. inspect service/API callers;
4. consider existing production rows;
5. verify zero/default/NULL behavior;
6. verify indexes and uniqueness constraints;
7. preserve SQLite/MySQL/PostgreSQL compatibility.

Do not rename or delete persisted fields, columns, enum values, or constraints as cleanup without an explicit compatibility plan.

A Go struct change is not automatically a safe database migration.

## Cross-Database SQL

Shared model code must work on SQLite, MySQL, and PostgreSQL unless explicitly backend-specific.

Prefer portable GORM clauses and simple SQL.

Be careful with:

- `ON CONFLICT` / upsert behavior;
- JSON functions;
- booleans;
- date/time functions;
- string concatenation;
- collations;
- `RETURNING`;
- generated columns;
- partial indexes;
- identifier quoting;
- full-text search.

If backend-specific SQL is required, branch explicitly by dialect and document/test it. Never fix one backend by silently breaking another.

For batch upserts with row-specific conflict updates, prefer GORM constructs that let the dialect rewrite incoming-row references correctly, such as `clause.Column{Table: "excluded", ...}` where supported by the current drivers.

## GORM and Schema Rules

Treat GORM tags as schema definitions.

Be deliberate with:

- `size`;
- `not null`;
- `default`;
- `index`;
- `uniqueIndex`;
- composite-index names and priorities;
- column types.

Reuse existing base models and ID conventions.

Do not add indexes blindly. Every index adds write, storage, and migration cost.

For a new hot query, inspect the real `WHERE`, `ORDER BY`, expected cardinality, and existing indexes before adding another one.

## Queries

Queries whose behavior depends on order must use an explicit `ORDER BY`.

This is mandatory for timelines, pagination, queues, recent items, pinned/priority items, exports, and cursor generation.

Avoid:

- unbounded scans on large tables;
- accidental N+1 queries;
- large preloads when only a few columns or counts are required;
- repeated queries for data already available to the caller;
- per-row queries inside hot loops when one batch query is equivalent.

Use explicit selects/joins when they materially reduce unnecessary work and remain readable.

Pagination must use deterministic ordering and stable tie-breakers.

## Updates and Concurrency

Do not `Save` a stale full model when only a few fields should change.

Prefer targeted:

- `Update`;
- `UpdateColumn`;
- `Updates(map[string]any{...})`;
- conditional updates with `RowsAffected`.

Remember that struct-based GORM `Updates` may omit zero values while map-based updates can explicitly write `false`, `0`, and `""`.

Use database uniqueness/constraints for invariants that must survive concurrent processes. A Go mutex only protects one process.

For compare-and-set behavior, include the expected state/version in the `WHERE` clause and inspect `RowsAffected`.

## Transactions

Use transactions only when an invariant truly requires atomic multi-write behavior.

Keep transaction boundaries narrow.

A helper expected to participate in a caller transaction must use the provided `*gorm.DB`; do not silently fall back to the global DB.

Do not add nested/independent transactions without understanding GORM and backend behavior.

Avoid holding a transaction or SQLite writer while doing unrelated work, network I/O, large loops, or expensive computation.

Do not wrap many independent hot-path statements in one large transaction merely to reduce commit count; measure the actual contention first.

## Hooks

Use GORM hooks sparingly.

Hooks must stay persistence-local and lightweight.

Do not hide in hooks:

- WebSocket broadcasts;
- notifications;
- external HTTP calls;
- permission workflows;
- unrelated model mutations;
- request-context behavior.

Hooks run in migrations/tests and can silently amplify hot-path database work.

When optimizing a create-only hook, preserve update/replace semantics of reusable helpers. Example: a new message with no image attachment may skip attachment synchronization, but the general replace helper must still delete old attachment rows when an existing message changes from images to plain text.

## Deletes

Understand whether the model uses hard delete, soft delete, archival, or status flags.

Do not change one lifecycle semantic into another casually.

Bulk updates/deletes must use narrow predicates. Never add an unscoped destructive mutation without an explicit requirement.

When deleting persistent references, inspect dependent rows and external storage references.

## Time, NULL, and Persistent Values

Follow surrounding project conventions for time storage and comparison; use UTC where existing code does.

Distinguish database `NULL` from Go zero values when the difference matters.

Be especially careful with:

- `*time.Time`;
- optional strings;
- booleans with meaningful unset state;
- counters and versions;
- foreign IDs.

Persistent enum/string values are contracts. Do not rename them as cosmetic cleanup.

## Sensitive Data

Models may contain hashes, tokens, external IDs, notification endpoints, or configuration.

Do not expose sensitive fields through JSON unless explicitly intended.

Do not store plaintext secrets where the existing design stores hashes.

Do not add logging of secrets from model helpers.

## Performance Rules

Do not optimize by guesswork. For hot paths, measure:

- query count;
- write count;
- connection-pool waits;
- transaction duration;
- result cardinality;
- index use;
- duplicated work.

Prefer reducing redundant database work before increasing connection-pool size.

For SQLite in particular, a larger `MaxOpenConns` does not create multiple concurrent writers. It may move waiting from `database/sql` into SQLite lock contention.

Do not change the current SQLite connection-pool defaults without a controlled A/B test that checks both:

- `database/sql` wait count/duration;
- SQLite busy/lock errors and tail latency.

Batch equivalent writes when business semantics are unchanged. Avoid per-window/per-row statement amplification.

## Established Hot-Path Invariants

The following optimizations are intentional and should not be casually reverted.

### Digest windows

Digest recording uses seven supported windows, but database writes are batched.

For one normal message:

- visitor rows: at most one batch upsert;
- speaker rows: at most one batch upsert.

Do not restore seven independent visitor plus seven independent speaker statements.

Speaker conflict updates must preserve:

- `message_count = message_count + 1`;
- existing `first_message_at`;
- incoming `window_end`;
- incoming `speaker_display_name`;
- incoming `last_message_at`;
- `updated_at`.

Digest visit recording should insert all supported visitor windows in one batch.

### Message image attachments

For a newly created message with no image attachment IDs, `AfterCreate` should return without opening the attachment replacement transaction.

Do not move that fast-path into the generic replacement helper: existing-message updates from image content to plain text must still delete old attachment rows.

### App notification persistence helpers

Do not reintroduce repeated preference-table scans in the message hot path.

The combined external-notification preference query is intended to represent the union of valid ServerChan, Bark, and Meow consumers.

Avoid adding per-message database lookups when the same already-loaded data can be reused by the service layer.

### Webhook/event-log helpers

Keep persistence helpers small and composable so callers that already know origin metadata do not need an unnecessary lookup merely to append an event log row.

Do not force all event-log writes through a helper that always re-queries external references.

## Testing

For model changes, add focused persistence tests when practical.

Important cases include:

- defaults and normalization;
- unique constraints;
- composite uniqueness;
- zero-value updates;
- nullable fields;
- deterministic ordering;
- pagination boundaries;
- conflict/upsert semantics;
- create/update/delete lifecycle behavior;
- batch behavior;
- migration compatibility.

For cross-database code, do not rely on SQLite-only SQL semantics unless the implementation is intentionally SQLite-specific.

Use existing project test DB setup instead of inventing a parallel harness.

Run focused tests first:

```bash
go test ./model
```

When service/API behavior consumes the change:

```bash
go test ./service
go test ./api
```

When practical:

```bash
go test ./...
```

For concurrency-sensitive code, consider:

```bash
go test -race ./model ./service
```

Do not claim tests passed unless they were actually executed.

## Review Checklist

Before finishing a model/database change, verify:

- no unintended schema/table/column rename;
- no changed default or NULL semantics;
- no accidental JSON exposure;
- no SQLite-only behavior in shared code;
- deterministic ordering where required;
- no stale full-model save;
- no broad update/delete;
- no unnecessary transaction;
- no N+1 or repeated hot-path query;
- no blind index addition;
- no side effect hidden in a hook;
- no regression of the established hot-path invariants above;
- focused tests cover the changed persistence semantics.

Persistence changes should be conservative, measurable, and explicit.
