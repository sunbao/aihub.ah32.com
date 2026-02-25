## 1. Catalog Data + APIs

- [x] 1.1 Define initial Agent Card catalogs (persona/interests/capabilities/bio/greeting) and sample dataset files
- [x] 1.2 Add user-auth API to list approved persona templates for selection
- [x] 1.3 Add user-auth API to fetch Agent Card catalogs with `catalog_version` for caching
- [x] 1.4 Add lightweight client-side caching strategy in `/app/` for catalogs (version-based)

## 2. Backend Card Validation + Moderation Gates

- [x] 2.1 Enforce `card_review_status=approved` in anonymous agent discovery endpoints
- [x] 2.2 Implement server-side “guided authoring” validation against catalogs for interests/capabilities/bio/greeting/persona template id
- [x] 2.3 Auto-approve cards that use only catalog selections; mark cards pending when custom values are present
- [x] 2.4 Ensure card updates invalidate prior certification consistently and clear stale OSS sync metadata

## 3. Mobile `/app/` Wizard UX

- [x] 3.1 Replace free-form editor with a step-by-step wizard (persona → personality preset → interests → capabilities → bio/greeting → review)
- [x] 3.2 Add persona template picker (pulls from persona templates API) and show anti-impersonation disclaimer
- [x] 3.3 Replace interests/capabilities free-text inputs with searchable multi-select chips backed by catalogs
- [x] 3.4 Add bio/greeting template selector (and optional advanced custom editor that clearly requires review)
- [x] 3.5 Surface `card_version` + `card_review_status` + “Sync to OSS allowed” in the wizard summary

## 4. UI Card Visibility Improvements

- [x] 4.1 Update Square card UI to show a short profile snippet + interests + personality hint
- [x] 4.2 Update Agent detail UI to render greeting/personality/persona summary when present

## 5. Verification + Docs

- [x] 5.1 Add/update smoke steps documenting how to complete a Card via wizard and sync to OSS
- [x] 5.2 Run `openspec validate` for the change and fix any formatting issues
