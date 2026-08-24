# SealChat Chat Subsystem Agent Guide

This file defines instructions for AI coding agents working under `ui/src/views/chat/`.

Repository-wide `AGENTS.md` and `ui/AGENTS.md` also apply.

The chat subsystem is a compatibility-sensitive integration surface. Changes here can affect message ordering, live synchronization, editing, input state, embeds, theater mode, identities, attachments, and multiple auxiliary tools.

## Scope

This directory includes the main chat view and chat-specific:

- components;
- composables;
- helpers;
- message rendering behavior;
- input behavior;
- identity interactions;
- search/history navigation;
- live message synchronization;
- auxiliary chat panels.

Do not assume a function is local in effect merely because it is defined in `chat.vue`.

Search for event producers, store mutations, route dependencies, and consumers before changing behavior.

## Core Invariants

Unless the task explicitly requires otherwise, preserve the following behavior.

### Channel Context

- state must correspond to the currently active channel/world;
- results from an old channel must not overwrite the new channel;
- channel switches must not leak previous-channel transient state;
- initialization and cleanup ordering must remain coherent.

### Message Ordering

- messages must remain deterministically ordered;
- duplicate realtime/history messages must not produce duplicate rows;
- edits must update the intended row;
- deletion/revocation must remove or transform the intended row;
- pinned and normal message representations must remain consistent where applicable.

Do not casually replace existing ordering/cursor logic with timestamp-only assumptions.

### Pagination and History

Preserve:

- older-message loading;
- newer-message loading where supported;
- cursor progression;
- reached-start/reached-latest semantics;
- history-mode entry/exit;
- scroll anchoring;
- search-jump behavior;
- first-unread behavior.

Do not mix cursor semantics from different query modes without verifying the backend contract.

### Realtime and Resume

Preserve behavior for:

- incoming messages;
- message updates;
- message deletion;
- reconnect;
- foreground resume;
- page restore;
- channel-context clearing.

Avoid double registration of realtime handlers.

A reconnect or resume path must not race with a stale fetch and overwrite newer channel state.

### Composer State

Preserve:

- plain/rich input mode;
- IC/OOC mode;
- whisper state;
- editing state;
- input history;
- draft/session restoration;
- inline attachment markers;
- selection/cursor behavior;
- keyboard shortcuts;
- send permissions.

Do not change send-time normalization order without tracing the complete pipeline.

### Editing

Editing is distinct from normal sending.

Preserve:

- target message identity;
- channel ownership;
- save/cancel behavior;
- editing preview behavior;
- re-edit/revoke behavior;
- state cleanup when the channel changes.

Do not let stale editing state survive into another channel.

### IC/OOC and Identity

Treat IC/OOC and channel identity state as domain behavior, not presentation-only state.

Preserve:

- active identity;
- display-name/avatar resolution;
- identity variants;
- IC/OOC mapping;
- temporary identities;
- identity-specific appearance;
- theater presentation linkage where used.

Before changing identity data, inspect all relevant stores, migration helpers, and rendering paths.

### Whisper

Whisper state is compatibility-sensitive.

Preserve:

- target selection;
- restoration from draft/history where supported;
- message payload semantics;
- UI mode transitions;
- channel-change cleanup.

### Mentions and Commands

Preserve existing semantics for:

- `@` mention suggestions;
- mention serialization;
- command suggestions;
- command-like content detection;
- stale async mention searches.

Do not replace structured mention markup with plain display text.

### Typing and Editing Preview

Typing/editing preview has its own lifecycle.

Preserve:

- broadcast mode;
- preview content visibility rules;
- ordering;
- self/remote distinction;
- editing/typing distinction;
- drag ordering if enabled;
- cleanup on unmount/channel changes;
- throttling/debouncing semantics.

Do not register duplicate pointer/window/chat-event listeners.

### Attachments and Images

Preserve:

- attachment IDs;
- attachment URL resolution;
- upload semantics;
- image insertion behavior;
- image send behavior;
- drag/drop formats;
- gallery references.

Do not convert attachment IDs to raw URLs in persisted message content unless explicitly required.

### Observer Mode

Observer/read-only restrictions must remain enforced.

Do not enable send/edit/manage actions merely because a UI control becomes accessible.

Check both UI gating and underlying operation behavior where relevant.

### Embed and Theater Modes

Chat is used in alternate presentation contexts.

Preserve route-dependent behavior for:

- `/embed`;
- theater embeds;
- theater view;
- split view where chat interactions are reused.

Do not assume the normal chat route is the only rendering environment.

## Event Ownership

`chatEvent` and other event sources are compatibility-sensitive.

Before changing an event:

1. find its emitters;
2. find all listeners;
3. verify payload shape;
4. verify registration timing;
5. verify cleanup.

When adding a listener, pair it with cleanup if the listener is component-scoped.

Avoid wildcard removal unless existing code intentionally relies on it.

Do not rename events casually.

## Async Context Safety

Many operations depend on channel/world state that can change while a request is running.

For async operations that write chat state:

- capture relevant IDs/signatures at start;
- verify they are still current before committing results;
- preserve existing request sequence/epoch protections;
- avoid stale responses overwriting newer state.

This applies especially to:

- message loading;
- search;
- member/mention loading;
- identity loading;
- image/gallery loading;
- character-related refreshes;
- reconnect/resume flows.

## Scroll and DOM State

Scroll behavior is user-visible state.

Be careful when changing:

- `nextTick` placement;
- list mutations;
- virtualized rendering;
- scroll-to-bottom logic;
- history anchoring;
- observers;
- sentinel placement.

Do not assume list mutations and DOM layout complete synchronously.

Avoid changing scroll behavior as a side effect of unrelated work.

## Message Rendering

Message rendering supports multiple content forms and display modes.

Preserve:

- rich text rendering;
- plain text rendering;
- mentions;
- bot-command rendering;
- keyword highlighting;
- spoilers;
- text decoration;
- attachments;
- identity presentation;
- message merge rules.

Do not bypass existing sanitization or escaping.

Do not move rendering responsibilities between parent and `chat-item` without checking both normal and pinned message paths.

## Multi-select and Message Actions

Preserve selection semantics and display order.

Actions such as:

- copy;
- forward;
- archive;
- delete;
- copy-as-image

must operate on the selected messages intended by the current channel/context.

Do not use stale row snapshots after channel changes.

## Feature Panels

Chat hosts multiple auxiliary features.

When changing a panel, prefer keeping its domain behavior inside its existing component/store/composable rather than pushing additional independent state into the main page.

Panel visibility state may remain page-owned when it is purely orchestration.

Do not make unrelated panels depend on each other's local state.

## Styling

Chat styling includes both scoped and global rules.

Preserve the distinction.

Global chat styles may exist because they target:

- Teleport content;
- third-party component DOM;
- TipTap-generated DOM;
- virtual-list internals;
- global rich-content classes.

Do not convert global selectors to scoped selectors without verifying the rendered DOM.

Do not change class names during unrelated behavior work.

Keep responsive behavior for desktop and mobile intact.

## File Placement

Use existing chat-local structure where appropriate:

- `components/` for chat UI regions;
- `composables/` for chat-specific state/effect lifecycles;
- small local modules for pure domain helpers.

Shared project-wide behavior should live outside the chat directory when it is genuinely reusable.

Do not duplicate a project-level utility inside chat merely for convenience.

## Validation

For any meaningful chat change, run:

```bash
cd ui
npm run type-check
npm run build
```

Compilation is necessary but not sufficient for stateful chat changes.

When the change touches the relevant area, verify or reason explicitly about the affected subset of:

- entering a channel;
- switching channels;
- sending;
- receiving realtime messages;
- loading older messages;
- history mode;
- search jump;
- first unread;
- editing;
- revoke/re-edit;
- IC/OOC;
- whisper;
- mentions;
- commands;
- typing preview;
- editing preview;
- attachments;
- emoji/gallery;
- multi-select;
- reconnect/resume;
- observer mode;
- embed mode;
- theater mode.

Do not claim manual smoke verification unless it was actually performed.

## Diff Review

Before finishing, inspect the final diff and specifically look for:

- duplicated listeners;
- missing cleanup;
- stale async writes;
- changed event payloads;
- changed message ordering;
- accidental state reset changes;
- changed persisted keys;
- template/CSS churn;
- unrelated chat features modified by formatting.

Keep chat changes narrowly scoped because regressions often appear outside the immediately visible UI.
