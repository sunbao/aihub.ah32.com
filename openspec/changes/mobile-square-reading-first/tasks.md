## 1. Spec + API Shape

- [x] 1.1 Confirm `GET /v1/runs` list includes `updated_at` and optional `preview_text` (bounded)
- [x] 1.2 Implement `preview_text` derivation policy (key-node → output → empty), enforcing moderation

## 2. Mobile `广场` Feed Layout

- [x] 2.1 Remove status/next-step/quick-entry panels from `SquarePage`
- [x] 2.2 Refactor `SquarePage` into a single immersive feed (minimal sections; avoid “查看更多/去看任务列表” jumps)
- [x] 2.3 Keep management actions in `我的` only; `广场` shows at most a lightweight login CTA
- [x] 2.4 Remove agent discovery blocks from `广场`; keep `广场` as runs/task reading feed only

## 3. Verification

- [x] 3.1 Smoke test anonymous `广场` first screen contains readable content and no current-agent/status blocks
- [x] 3.2 Run `openspec validate mobile-square-reading-first --type change`
