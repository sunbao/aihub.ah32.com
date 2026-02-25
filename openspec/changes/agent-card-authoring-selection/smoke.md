# Smoke test: Agent Card wizard + OSS sync

## Prereqs
- Backend running locally
- `/app/` accessible (mobile UI)
- A valid user API key (login)
- OSS configured if you want to test real sync (otherwise you can still validate card review gating)

## 1) Wizard “pure selection” path (auto-approved)
1. Open `/app/` → `我的` → create or select an agent.
2. Click `编辑 Agent Card` to open the wizard.
3. Complete steps using **only selections**:
   - Persona: select an approved template (or leave unset).
   - Personality preset: pick one preset.
   - Interests: pick several from the catalog.
   - Capabilities: pick several from the catalog.
   - Bio: pick a template (do NOT enable custom) and keep the rendered result.
   - Greeting: pick a template (do NOT enable custom) and keep the rendered result.
4. Save.
5. Confirm in the wizard summary that:
   - `card_review_status` becomes `approved`
   - “Sync to OSS allowed” is `是`

## 2) Custom content path (requires review; blocks OSS sync)
1. In the wizard step “简介与问候”, enable `高级：自定义` for bio or greeting.
2. Edit the text and Save.
3. Confirm:
   - `card_review_status` becomes `pending`
   - “Sync to OSS allowed” is `否`
4. Attempt `同步到 OSS`:
   - Expect HTTP 412 with error `agent card not approved`

## 3) Public discovery gate
1. Open `/app/` in an anonymous session (no user API key).
2. Visit `广场` and confirm discovered agents only include those with `card_review_status=approved`.
