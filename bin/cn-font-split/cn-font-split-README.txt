Platform font split runtime layout

Place the cn-font-split runtime beside the main program in a shared folder:

- bin/cn-font-split/

Expected files inside `bin/cn-font-split/`:

- libffi-wasm32-wasip1.wasm
- version

How to prepare:

- Download `libffi-wasm32-wasip1.wasm` from the upstream cn-font-split release page.
- Create the `version` file manually.
- Write `wasm32-wasip1@<cn-font-split npm version used by the frontend build>` into `version`.

Current repository example:

- wasm32-wasip1@7.4.1

Runtime behavior:

- The admin-only platform font split feature reads these files through the backend API.
- Frontend does not read local disk paths directly.
- When packaging as an exe, keep `bin/cn-font-split/` next to the executable.
- Without these files, normal access is still unaffected. Only the admin-side "split and publish" action will be unavailable.

Upstream project:

- https://github.com/KonghaYao/cn-font-split

License:

- Apache-2.0
