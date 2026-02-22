# Project rules

## 1) File size

- Any source file over **2000 lines** must be split into smaller, focused files.

## 2) Error handling

- Do **not** swallow errors (no silent ignores).
- Avoid nested error handling; prefer early returns.
- Every error must be logged.
- For user-facing failures, return a clear error response/message and notify the user when appropriate.

