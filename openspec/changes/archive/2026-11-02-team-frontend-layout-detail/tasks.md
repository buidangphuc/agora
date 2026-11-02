## 1. Layout

- [x] 1.1 Rebuild the PDP header as a two-column grid with a sticky gallery and a padded buy-box `Card`; update `loading.tsx`, `error.tsx`, `not-found.tsx`; verify `npx tsc --noEmit`

## 2. Blocks

- [x] 2.1 Restyle gallery, anchor nav, variant chips, shipping, specs, share, report and alert controls with 44 px targets; verify the scoped vitest files pass
- [x] 2.2 Restyle the Q&A and review blocks; verify the scoped vitest files pass

## 3. Verification

- [x] 3.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 3.2 Playwright overflow and touch-target check at 375, 768 and 1280 px on the production build
- [x] 3.3 `openspec validate team-frontend-layout-detail --strict`
