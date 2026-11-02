## 1. Platform

- [x] 1.1 Dialog controlled without trigger, optional description, footer, size, closeLabel, native `dialog` with `aria-modal`; tokens, touch-target test, ui test, state, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facade

- [x] 2.1 Rewrite `Modal.tsx` as a facade over the platform `Dialog` keeping the Modal props; rewrite `Modal.test.tsx` (name, description, footer inside, Escape, close button, overlay, focus return); verify the facade tests pass

## 3. Call sites

- [x] 3.1 Confirm every `Modal` call site in `team-frontend/src` (features, seller, admin, dev catalogue) passes a string title and compiles unchanged; verify `tsc` has no errors
- [x] 3.2 Scan `@fe-platform/ui` and the tokens class maps in `tailwind.config.ts`; verify the built CSS contains the dialog utilities

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 Check platform-e2e dialog selectors (`get_by_role("dialog")`, `dialog[open]`, buttons inside the dialog) against the new markup
- [x] 4.3 `openspec validate team-frontend-dialog --strict`
