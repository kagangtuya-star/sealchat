# SealChat Frontend Agent Guide

This file defines frontend-specific instructions for AI coding agents working under `ui/`.

Repository-wide rules from the root `AGENTS.md` also apply.

## Scope

The frontend is a Vue 3 + TypeScript application built with Vite.

Primary stack:

- Vue 3
- TypeScript
- Pinia
- Naive UI
- Vue Router
- VueUse
- SCSS
- Vite

This file applies to all code under `ui/`.

Feature-specific rules may exist in deeper `AGENTS.md` files and take precedence within their directories.

## Frontend Architecture

Use existing project boundaries before introducing new ones.

Prefer:

- page/view components for route-level orchestration;
- components for coherent visual or interaction regions;
- composables for cohesive state/effect lifecycles;
- Pinia stores for genuinely shared application state;
- services for reusable application/domain operations;
- utilities for stateless reusable transformations.

Do not move state into Pinia merely to make a component shorter.

Do not create a composable solely to wrap one trivial expression or one existing function.

Do not pass a giant page-level context object into a composable. If a composable requires many unrelated refs and callbacks, reconsider its responsibility boundary.

## Vue Conventions

Use Composition API.

Prefer `<script setup lang="ts">` unless the existing file intentionally uses another form.

Keep reactive ownership clear:

- `ref` / `reactive` for local mutable state;
- `computed` for derived state;
- stores for shared state;
- props/emits for parent-child contracts.

Avoid hidden cross-component mutation.

Do not mutate props.

Do not introduce `provide/inject` as a shortcut around explicit ownership unless the state is genuinely scoped to a component subtree.

When exposing component methods through refs, keep the public surface small and intentional.

## TypeScript

Prefer explicit domain types over `any`.

Do not replace existing strong types with `any` merely to make a refactor compile.

When the external API is weakly typed, normalize at the boundary where practical.

Reuse existing types from:

- `@/types`
- stores
- services
- protocol packages
- feature-local type modules

before defining duplicates.

Avoid broad type assertions when a narrow runtime check can establish the type.

Do not hide genuine type errors with `as any` unless the surrounding code already requires it and there is no reasonable alternative.

## State Ownership

Before adding state, decide who owns it.

Use local component/composable state when the value is:

- tied to one mounted UI;
- disposable with that UI;
- not required by unrelated routes/components.

Use a Pinia store when the value is:

- shared across independent components;
- expected to survive local component replacement;
- part of application-level state;
- already owned by an established store domain.

Do not create duplicate local caches of store-owned state without a concrete reason.

## Async and Race Safety

Frontend state frequently depends on changing world, channel, route, user, or session context.

For async work:

- capture the relevant context at request start;
- reject or ignore stale responses when context has changed;
- preserve current request ordering semantics;
- avoid earlier requests overwriting newer state;
- avoid starting duplicate requests where existing code already serializes or deduplicates them.

When an operation may outlive its component or context, explicitly consider cancellation, stale guards, or cleanup.

## Lifecycle and Cleanup

Treat lifecycle ownership as part of the feature.

When adding or moving:

- `watch`
- `watchEffect`
- `onMounted`
- `onBeforeUnmount`
- `useEventListener`
- `setTimeout`
- `setInterval`
- throttled/debounced functions
- observers
- WebSocket listeners
- application event listeners
- document/window pointer or keyboard listeners

make sure registration and cleanup remain paired.

Do not register the same listener from multiple ownership layers unless duplicate handling is intentional.

Cancel or flush throttled/debounced work according to existing behavior when its owner is destroyed.

## Stores and Events

Reuse existing Pinia stores and application event channels.

Do not create a second event bus when an established event mechanism already exists.

When listening to an event:

- confirm the event name and payload shape from the producer;
- preserve existing subscription timing;
- unregister listeners during cleanup when required.

When emitting an event:

- search for all consumers before changing payload semantics.

## Components

Extract a component when it owns a coherent UI responsibility.

Good reasons include:

- a distinct interaction region;
- independent lifecycle;
- repeated UI;
- a clear props/emits contract;
- significant template complexity with a stable boundary.

Do not split markup into many tiny components solely to reduce file length.

Avoid components that simply forward a large set of props and emits without owning any meaningful behavior or presentation boundary.

## Composables

A composable should own a cohesive behavior.

A well-bounded composable may own:

- local state;
- derived state;
- async operations;
- watchers;
- listeners;
- lifecycle cleanup;
- feature-specific helpers.

Keep inputs explicit.

Keep outputs intentional.

Do not turn composables into generic bags of unrelated page state.

If several composables repeatedly require the same large unrelated dependency set, reconsider the domain design rather than creating an implicit global context.

## API Calls

Use the project's established API/store/service layer.

Do not create ad hoc `fetch` or Axios calls inside components if an existing API abstraction already owns that domain.

Preserve:

- endpoint paths;
- request field names;
- response normalization;
- authorization behavior;
- error handling conventions.

When backend response shapes are inconsistent, normalize them at one clear boundary rather than scattering compatibility checks across many components.

## Routing

Use Vue Router through the project's established patterns.

Do not manually manipulate browser location when router navigation is appropriate.

Preserve route/query compatibility.

When route-dependent async work is performed, guard against the route changing before completion.

## UI Library

Prefer existing Naive UI patterns.

Before introducing custom controls, check whether the project already has:

- a shared component;
- a Naive UI-based pattern;
- a feature-local control that can be extended.

Do not replace working Naive UI components with custom implementations without a concrete requirement.

## Styling

The project uses SCSS together with scoped and global Vue styles.

Preserve selector scope.

Do not move a selector from scoped to global, or from global to scoped, without verifying the effect.

Be careful with:

- `:deep(...)`
- `:global(...)`
- Teleport-rendered content
- third-party component DOM
- CSS variables
- responsive media queries
- theme selectors
- night/custom palettes

For large style blocks, external `.scss` files are acceptable and often preferable, provided the original scope and cascade order are preserved.

Do not rename CSS classes during unrelated logic changes.

Do not reformat or reorder large style blocks unless style restructuring is the task.

## DOM and Browser APIs

Check browser support and cleanup when using:

- Clipboard API
- Drag and Drop
- File APIs
- ResizeObserver
- IntersectionObserver
- Pointer Events
- storage APIs
- fullscreen or media APIs

Do not assume DOM elements exist before mount or after unmount.

Guard direct DOM access when code can run in alternate route/embed states.

## Local and Session Storage

Treat persisted browser keys as compatibility-sensitive.

Do not rename or reinterpret an existing key without a migration or explicit compatibility decision.

When introducing a new key:

- use a project-specific prefix where appropriate;
- define a stable value format;
- handle missing or invalid values safely.

## User Content and Rich Text

Treat user-generated content as untrusted.

Preserve existing sanitization and escaping behavior.

Do not bypass DOMPurify or equivalent sanitization in flows that render user-controlled HTML.

Be careful when converting among:

- plain text;
- escaped text;
- HTML;
- TipTap JSON;
- attachment markers;
- mention markup;
- bot-command markup.

Do not normalize or rewrite user content unless the feature explicitly requires it.

## Performance

Do not optimize speculatively.

For hot paths such as message rendering, large lists, image galleries, typing updates, or frequently-fired watchers:

- avoid unnecessary deep watchers;
- avoid repeatedly traversing large arrays when an existing indexed path exists;
- preserve virtualization behavior;
- avoid creating new reactive objects in tight render loops without need.

Performance changes should preserve visible behavior.

## Dependencies

Do not change `package.json` or the lockfile unless the task explicitly requires a dependency change.

Reuse the existing frontend stack.

## Validation

For frontend changes, run:

```bash
npm run type-check
npm run build
```

from `ui/`.

If working from repository root:

```bash
cd ui
npm run type-check
npm run build
```

Run more focused validation first when available.

Do not claim browser behavior was verified unless it was actually exercised.

## Diff Review

Before finishing a non-trivial frontend task, inspect:

```bash
git diff --stat
git diff
```

Check specifically for:

- accidental template reformatting;
- CSS churn;
- duplicated imports;
- dead reactive state;
- stale listeners;
- unintended API changes;
- changed persisted keys;
- unrelated component modifications.

## Local Instructions

Some frontend subsystems define more specific constraints.

For example:

- `src/views/chat/AGENTS.md` — chat subsystem invariants and lifecycle rules.

Follow those local instructions in addition to this file.
