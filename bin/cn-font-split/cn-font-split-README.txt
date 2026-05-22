Platform font split runtime layout

Place the cn-font-split runtime beside the main program in a shared folder:

- bin/cn-font-split/

Expected files inside `bin/cn-font-split/`:

- libffi-wasm32-wasip1.wasm
- version

Example `version` content:

- wasm32-wasip1@7.6.8

Runtime behavior:

- The admin-only platform font split feature reads these files through the backend API.
- Frontend does not read local disk paths directly.
- When packaging as an exe, keep `bin/cn-font-split/` next to the executable.

Upstream project:

- https://github.com/KonghaYao/cn-font-split

License:

- Apache-2.0
