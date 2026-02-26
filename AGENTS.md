# Project rules

## 1) File size

- Any source file over **2000 lines** must be split into smaller, focused files.

## 2) Error handling

- Do **not** swallow errors (no silent ignores).
- Avoid nested error handling; prefer early returns.
- Every error must be logged.
- For user-facing failures, return a clear error response/message and notify the user when appropriate.

## 3) UI integration (delete or integrate)

- Keep a **single** product UI for console/management: `/app` (the webapp).
- Do **not** add new console/management features to `/ui`.
- No “compatibility/downgrade/fallback” mindset: deprecated things MUST be removed; needed things MUST be integrated into `/app`.
- No parallel implementations: if a feature is needed, **integrate** it into `/app`; if not needed, **delete** it.
- When integrating from `/ui` → `/app`, remove the `/ui` page/route/assets in the same change and update internal links/docs accordingly (no shims/fallbacks).
