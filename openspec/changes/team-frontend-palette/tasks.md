## 1. Codemod

- [x] 1.1 Write the mapping, run it as a dry run, review counts and leftovers, then apply to `team-frontend/src` (non-test code); verify `tsc`, `check-tokens` and biome are clean and the vitest suite is unchanged
- [x] 1.2 Rename the remaining `gray-N` to `neutral-N`, `shadow-shopee` to `shadow-card`, `shadow-orange-*` to `shadow-primary-*`; verify the grep for `gray-N`, `brand`, `shopee` in non-test source is empty

## 2. Gate

- [x] 2.1 Add `src/app/palette.test.ts`; verify it goes red when `text-gray-500 bg-brand` is added to a component and green again after reverting

## 3. Verification

- [x] 3.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 3.2 Serve the built app, compare computed colours of converted elements on home, search, product, cart and the chat shell before and after, and record the findings
- [x] 3.3 Run the B1 e2e colour scenarios against the local app

## 4. Done-condition

- [x] 4.1 `openspec validate team-frontend-palette --strict`, fe-platform `npm run check`, `repo-doctor` scope check; the report lists the classes left on purpose
