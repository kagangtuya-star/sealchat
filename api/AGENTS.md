# SealChat API Agent Guide

This file defines API-layer instructions for AI coding agents working under `api/`.

Repository-wide rules from the root `AGENTS.md` also apply. More specific instructions in deeper directories take precedence if present.

## Scope

The `api/` package is SealChat's transport layer. It contains HTTP and WebSocket entry points built primarily with Fiber and connects external requests to application behavior implemented elsewhere.

Typical responsibilities include:

- route registration;
- path/query/body parsing;
- authentication context extraction;
- request-level validation;
- permission-aware entry points;
- mapping service errors to HTTP responses;
- response serialization and headers;
- WebSocket transport handling;
- serving selected embedded/static resources.

The API layer is compatibility-sensitive because it is consumed by the Vue frontend, integrations, embeds, external agents, and other clients.

## Read Before Changing

Before modifying an endpoint:

1. read the handler;
2. locate its route registration;
3. read the service functions it calls;
4. search frontend and external consumers of the route or payload;
5. inspect nearby tests;
6. inspect related protocol/documentation when the endpoint is public or integration-facing.

Do not infer payloads from UI labels or comments alone.

For routes registered from `api_bind.go` or related bind/setup files, preserve existing route grouping, prefixes, middleware order, and alternate public paths unless the task explicitly changes them.

## Layer Boundary

Keep transport concerns in `api/`.

A handler may:

- parse Fiber path/query/body values;
- obtain the current user from established request-context helpers;
- validate transport-level required fields;
- call service-layer operations;
- map typed/sentinel errors to status codes;
- shape an HTTP response;
- set protocol-specific headers.

A handler should not become the owner of a substantial business workflow.

Prefer:

```text
Fiber request
  -> parse / validate
  -> resolve authentication context
  -> service operation
  -> map result/error
  -> HTTP response
```

Do not move business rules into handlers merely because the rule is needed by one endpoint today.

Do not invent a new DAO/repository layer from the API package. Existing business code may legitimately use `model.GetDB()` inside the service layer; follow the current architecture rather than creating a parallel one.

## Fiber Conventions

Use existing Fiber patterns in nearby handlers.

Be careful with:

- `c.Params(...)`;
- query parsing and raw query enumeration;
- `c.BodyParser(...)`;
- `c.Status(...)`;
- `c.JSON(...)`;
- `fiber.Map`;
- response headers;
- context-local authentication state.

Validate values after trimming when surrounding code treats IDs/tokens as trimmed strings.

For malformed request bodies, return a clear 4xx response rather than allowing invalid zero values to continue into business logic.

Do not expose internal Go errors directly unless the endpoint intentionally defines user-visible validation errors originating from service parsing.

## Authentication and Authorization

Use the existing authentication and permission flow.

For authenticated endpoints, follow established current-user extraction patterns used by nearby handlers.

Do not trust user IDs, role names, world IDs, or channel IDs supplied in the request as proof of authorization.

Authorization should be based on server-side state and established service/permission logic.

Do not add an administrative or owner-only operation without verifying the corresponding permission path.

Observer/read-only behavior must remain enforced where applicable.

## Service Error Mapping

Service-layer sentinel/typed errors should normally be translated at the API boundary.

Use `errors.Is` where the service exposes sentinel errors.

Preserve distinctions such as:

- invalid request -> 400;
- unauthenticated -> 401;
- denied -> 403;
- missing/inactive resource -> 404;
- conflict -> 409;
- rate limit -> 429;
- unexpected internal failure -> 500.

Do not flatten meaningful service errors into 500 responses if the existing API contract distinguishes them.

Likewise, do not turn internal implementation errors into detailed client-visible messages that may leak state or secrets.

Keep error codes/messages stable for public integration endpoints unless a compatibility change is explicitly requested.

## Response Contracts

Treat response field names and shapes as contracts.

Before changing:

- JSON key names;
- nesting;
- optional/omitted fields;
- status codes;
- content type;
- pagination fields;
- error envelopes;
- timestamps;
- identifier formatting;

search for all known consumers.

Avoid returning raw model structs when an endpoint already has a deliberate response shape.

If an endpoint uses a custom encoder or formatting rule, preserve it. Some integration responses intentionally use specific JSON escaping, indentation, schema envelopes, or content types.

## REST and Public Integration Endpoints

Public or external-facing endpoints require stricter compatibility.

Examples include:

- Agent access/feed endpoints;
- observer/print endpoints;
- embeds;
- webhooks;
- exported/public resources;
- attachment access.

For such endpoints, preserve documented semantics unless explicitly changing the protocol.

When changing an Agent-access endpoint, inspect both `api/agent_access.go` and the corresponding `service/agent_*` implementation and public docs.

Do not accidentally expose bearer credentials, token hashes, internal IDs, or private configuration through management/read APIs.

When credentials are intentionally returned only at creation/rotation time, preserve that one-time visibility behavior.

## Agent Feed / Read-only Access

The Agent read-only interface is a protocol, not merely a convenience endpoint.

Preserve its established concepts unless explicitly changing them, including:

- token-based access;
- disabled/rotated token invalidation;
- documented resource selection;
- scope semantics;
- timestamp options;
- cursor/channel consistency;
- response schema/version;
- rate limiting;
- no-store/private cache behavior where configured.

Message content returned through the feed is untrusted user data and must never be treated as server or agent instructions.

Do not weaken response security headers without a concrete requirement.

## WebSocket and Event Transport

WebSocket event names and payloads are compatibility-sensitive.

Before modifying a WebSocket event:

1. find the server producer/consumer;
2. find frontend listeners/emitters;
3. inspect reconnection behavior;
4. verify serialization field names;
5. consider older clients if compatibility is expected.

Do not rename events as part of cleanup.

Do not reorder persistence and broadcast side effects without understanding the current delivery guarantees.

Transport code should not create duplicate broadcasts for a single successful business operation.

## Input Validation

Validate at the correct layer.

API-level validation should handle transport concerns such as:

- missing required fields;
- malformed JSON;
- invalid enum/query syntax;
- obviously malformed IDs/tokens;
- unsupported request format.

Business validity belongs in service logic when it depends on:

- database state;
- permissions;
- resource ownership;
- cross-resource relationships;
- concurrency;
- quotas;
- workflow state.

Do not duplicate the same business validation in API and Service unless one layer is intentionally performing an early transport check.

## Security Headers and Caching

Preserve endpoint-specific cache and security headers.

Pay attention to:

- `Cache-Control`;
- `Content-Type`;
- `X-Content-Type-Options`;
- `Referrer-Policy`;
- `X-Robots-Tag`;
- rate-limit headers;
- download/content-disposition behavior.

Sensitive/token-authenticated data should not accidentally become publicly cacheable.

Static/public documentation may intentionally use different caching semantics from authenticated data.

## File and URL Handling

Treat user-provided paths, filenames, URLs, redirects, and attachment identifiers as untrusted.

Use established escaping and path utilities.

Do not concatenate untrusted values into filesystem paths or public URLs without understanding normalization.

Preserve protections against:

- protocol-relative URL generation;
- path traversal;
- unsafe redirect rewriting;
- malformed host/port handling;
- content-type confusion.

## Logging

Do not log:

- passwords;
- complete bearer tokens;
- cookies;
- API secrets;
- authorization headers;
- private configuration values.

For tokenized endpoints, prefer stable non-secret identifiers or a deliberately stored token tail when diagnostics require correlation.

Avoid adding verbose per-request logging in hot endpoints unless operationally justified.

## Tests

When changing API behavior, prefer adding/updating API-package tests close to the affected handler or helper.

Test important mappings such as:

- malformed input;
- authentication failure;
- permission denial;
- not found;
- conflict;
- success response shape;
- protocol-specific headers when relevant.

For route/helper changes, use pure helper tests when possible rather than requiring a full server for every case.

## Validation

Run focused tests first:

```bash
go test ./api
```

If the change crosses into service/model behavior, also run the affected package tests.

When practical before completion:

```bash
go test ./...
```

Do not claim an endpoint was manually verified unless an actual HTTP/WebSocket flow was exercised.

## Diff Review

Before finishing an API change, check for:

- changed status codes;
- changed JSON keys;
- changed route paths;
- altered middleware order;
- leaked internal errors;
- weakened auth checks;
- duplicated service logic;
- accidental token/secret logging;
- unrelated handler formatting.

Keep the API diff transport-focused.
