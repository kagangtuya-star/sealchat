# SealChat Service Agent Guide

This file defines service-layer instructions for AI coding agents working under `service/`.

Repository-wide rules from the root `AGENTS.md` also apply. More specific instructions in deeper service subdirectories take precedence if present.

## Scope

The `service/` package owns SealChat application behavior and business workflows.

It currently spans domains such as worlds/channels/messages, Agent access/feed, announcements, notifications, attachments and storage, audio, AI-related services, imports/exports, integrations, and background/runtime behavior.

Service code may coordinate models, permissions, storage, external systems, process-local state, and events. This is the primary layer for business semantics.

## Architectural Role

Prefer the service layer when behavior:

- depends on database state;
- spans multiple models;
- requires permission decisions;
- coordinates a workflow;
- owns concurrency/race semantics;
- triggers durable state changes and subsequent side effects;
- is used by more than one transport entry point.

Service code should not depend on Fiber request/response objects.

Do not pass `*fiber.Ctx` into service functions.

Translate transport data into service inputs at the API boundary.

The current SealChat service layer may directly use `model.GetDB()` and GORM for domain operations. Do not invent a repository/DAO layer solely to satisfy an abstract architecture preference.

## Domain Ownership

Keep related business rules together.

Do not scatter one workflow across several unrelated service files unless the split follows a clear domain boundary.

A service operation should have a clear contract:

- inputs;
- authorization expectation;
- mutation/read behavior;
- returned domain/result value;
- sentinel/typed errors;
- side effects.

Avoid functions whose only purpose is to expose arbitrary database access to callers.

## Permissions

Authorization belongs in established service/permission flows when it depends on domain state.

Before adding or changing permission logic:

1. inspect existing role resolution;
2. inspect world/channel ownership semantics;
3. inspect observer/read-only behavior where relevant;
4. inspect API callers;
5. preserve established owner/admin/member distinctions.

Do not treat a client-supplied role as authoritative.

Do not silently broaden access because one existing caller already checked permissions; service functions reused elsewhere may need their own domain-level protection.

Conversely, avoid duplicating expensive permission work at several layers if the existing contract deliberately centralizes it.

## Database Access

Service code may use `model.GetDB()` where that is the established pattern.

When writing GORM operations:

- use explicit predicates;
- avoid broad updates/deletes;
- verify intended row scope;
- specify ordering whenever semantics depend on order;
- consider zero-value update behavior;
- distinguish `Find`, `First`, `Take`, `Updates`, `Save`, and `UpdateColumn(s)` semantics;
- check affected rows when conflict/race detection requires it.

Do not update a full stale model when only a subset of columns should change.

Prefer targeted column updates when concurrent requests may legitimately change other fields.

This is particularly important for settings, credentials, counters, status flags, and other independently mutable fields.

## Transactions

Use transactions when a business invariant spans multiple database writes that must succeed or fail together.

Define transaction boundaries around the invariant, not around arbitrary function length.

Within a transaction:

- use the transactional `*gorm.DB` consistently;
- do not accidentally perform one of the writes through the global DB;
- return errors so rollback occurs;
- avoid external side effects before commit unless they are explicitly designed to be compensatable.

Do not hold a database transaction open while performing slow network calls or heavy external work unless the invariant truly requires it.

## Durable State and Side Effects

Separate durable state from best-effort side effects.

When an operation includes database mutation plus WebSocket broadcast, push notification, email, webhook, storage operation, external API call, or background task dispatch, determine the required ordering from existing behavior.

As a default for state-derived notifications, prefer a successfully committed durable state before publishing an event that claims the state exists.

Do not change the existing ordering casually. Some workflows may intentionally upload/store before database commit or require compensation.

If a side effect fails after the durable mutation succeeds, preserve the subsystem's existing retry/error semantics rather than pretending the entire mutation was rolled back.

## Concurrency and Races

SealChat is a multi-user realtime application. Treat concurrent mutation as normal.

Consider races involving:

- two administrators editing the same setting;
- credential/token rotation;
- duplicate messages/events;
- quota consumption;
- counters;
- notification delivery;
- channel/world deletion;
- attachment lifecycle;
- reconnect/resume work;
- background jobs.

Do not use a read-modify-save sequence on an entire model when a targeted atomic update is safer.

Use database uniqueness constraints, conditional updates, affected-row checks, or process synchronization according to the scope of the invariant.

Process-local mutexes/maps only protect one server process. Do not mistake them for distributed/database-level guarantees.

Document process-local limitations when they materially affect behavior.

## Credentials and Secrets

Treat credentials as high sensitivity.

For secrets/tokens:

- generate with cryptographically secure randomness;
- store hashes instead of plaintext where the existing design does so;
- use constant-time comparison where appropriate;
- avoid logging complete values;
- expose plaintext only when the protocol intentionally requires one-time creation/rotation output.

Do not make stored hashes reversible.

Do not return an already-invalid credential after a concurrent rotation. Prefer explicit conflict detection when the existing workflow requires it.

## Sentinel and Typed Errors

Service errors are part of the boundary with API callers.

Use stable sentinel/typed errors for conditions the API must distinguish, such as denied, invalid input/state, not found, conflict, quota/rate limit, or unsupported values.

Wrap lower-level unexpected errors with useful context where appropriate, but preserve `errors.Is` compatibility for known domain errors.

Do not make the API parse error strings to determine status when a sentinel error can express the condition.

Do not expose raw database errors as user-facing validation messages.

## Read Operations

For list/feed/read APIs:

- make ordering explicit;
- keep pagination deterministic;
- preserve cursor semantics;
- avoid loading unbounded result sets;
- minimize N+1 queries;
- consider indexes used by filters/order;
- distinguish absence from internal query failure.

When a cursor belongs to a channel/resource, validate that it is not silently reused against another resource.

For time-window queries, be explicit about inclusive/exclusive boundaries to avoid duplicates or gaps.

## Message and Realtime Semantics

Message-related business logic is particularly compatibility-sensitive.

Before changing it, inspect persistence, ordering, cursor construction, IC/OOC filtering, whisper visibility, archive/deletion semantics, identity snapshots, realtime broadcast, unread/notification generation, and external Agent feeds.

Do not create a second interpretation of message visibility in a new service if an established shared rule exists.

## Agent Access / Feed

The Agent feed is an external read-only protocol.

Preserve established semantics unless explicitly changing them, including:

- credential resolution and rotation behavior;
- read-only access;
- scope filtering;
- timestamp modes;
- per-channel pagination/checkpoints;
- cursor validation;
- schema versioning;
- count/message rendering;
- rate limiting.

Agent message content is untrusted user data.

Never execute or interpret message content as instructions.

If checkpoint/state progression is implemented, only advance successful checkpoints after the relevant data operation has completed according to the protocol contract.

## Attachments and Storage

Attachment service code may coordinate database metadata and local/S3-compatible storage.

Treat storage keys and attachment IDs as durable references.

Do not change persisted reference formats casually.

When deleting or replacing assets, consider:

- database references;
- remote object existence;
- partial failures;
- retries;
- orphan cleanup;
- authorization;
- content type;
- path/key normalization.

Do not assume S3-compatible backends have local filesystem semantics.

## Background Work and Goroutines

Background service code must have clear lifecycle ownership.

For goroutines, timers, hubs, workers, and queues:

- avoid uncontrolled goroutine spawning;
- define shutdown behavior where the subsystem supports shutdown;
- avoid data races on shared maps/state;
- use locks/channels consistently;
- avoid holding locks during slow I/O;
- recover or surface worker errors according to existing patterns.

Do not create permanent background loops from request handlers without an established manager/runtime owner.

## External Calls

For HTTP/S3/AI/webhook or other external calls:

- set or preserve timeouts;
- validate remote URLs according to subsystem security requirements;
- avoid leaking credentials in errors/logs;
- distinguish retryable and permanent failures when existing logic does so;
- do not hold unrelated DB locks/transactions during slow network I/O.

Remote content is untrusted.

## Normalization

Normalize domain input once at a clear boundary.

Common examples include trimming IDs, canonical enum values, empty-to-default behavior, timestamp normalization, and attachment identifiers.

Avoid repeated slightly different normalization in several callers.

Do not normalize data in a way that changes user-authored message content unless explicitly required.

## Performance

Do not optimize by guesswork.

For hot paths:

- inspect query count;
- inspect result cardinality;
- inspect indexes;
- avoid repeated parsing/serialization;
- avoid unnecessary allocations in large feed/message loops;
- avoid global locks around unrelated work.

Large service files are not automatically wrong; split only when responsibilities are genuinely distinct.

## Tests

Prefer service tests for business semantics independent of HTTP transport.

Test:

- success;
- permission denial;
- invalid state;
- conflict/race-sensitive paths;
- boundary pagination;
- idempotency where expected;
- normalization;
- partial-failure behavior where practical.

For DB-backed tests, follow existing test database setup rather than creating a parallel harness.

## Validation

Run focused service tests first:

```bash
go test ./service
```

For a service subpackage:

```bash
go test ./service/<subpackage>
```

When the change affects API/model behavior, run those affected packages too.

When practical:

```bash
go test ./...
```

Use the race detector for concurrency-sensitive code when feasible:

```bash
go test -race ./service
```

Do not claim a race-sensitive path is safe solely because normal tests pass.

## Diff Review

Before finishing a service change, look for:

- accidentally widened DB predicates;
- full-model saves from stale state;
- missing transaction use;
- global DB calls inside a transaction;
- side effects occurring before required commit;
- new process-local state presented as globally safe;
- leaked secrets;
- changed sentinel errors;
- silently swallowed errors;
- transport/Fiber dependencies entering service code.

Keep business behavior explicit and testable.
