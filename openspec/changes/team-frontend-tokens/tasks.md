## 1. Config

- [x] 1.1 Rewrite the Tier 1 colours and legacy groups in `tailwind.config.ts` from `@fe-platform/tokens`, add `gray = neutral`, shadows, `min-h-touch`, safe-area utilities and the Tier 2 plugin; remove the `:root` block from `globals.css`; verify `npx tsc --noEmit` and a Tailwind build of a probe class set

## 2. Tests

- [x] 2.1 Update `src/app/tokens.test.ts` (platform-derived expectations plus the new checks); verify green, then verify red when `neutral` is reverted to the old gray values and when a dark block is imported
- [x] 2.2 Update the three e2e feature lines (`ui_foundation.feature`, `ui_token_gates.feature`); verify the features check (`scripts/features.py --strict` with the e2e venv) still passes

## 3. Verification

- [x] 3.1 `npm run check` and `npm run build` in `team-frontend`; clean-copy `npm ci` + check + build + `docker build`
- [x] 3.2 Serve the built app, read computed styles in the browser (header button background and `--color-action-primary`), and look at home, search, product and cart for leftover orange or low-contrast text; record findings
- [x] 3.3 Run the two updated e2e features against the locally started app if the harness allows; otherwise record exactly why not

## 4. Done-condition

- [x] 4.1 `openspec validate team-frontend-tokens --strict`, fe-platform `npm run check`, team-frontend `npm run check`, `repo-doctor`; the report lists leftover raw palette and legacy classes (B2, B3) honestly
