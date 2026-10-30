## 1. Layout

- [x] 1.1 Rework the checkout shell header and footer (spacing, radius, 44 px links); verify `src/app/(checkout)/layout.test.tsx` passes
- [x] 1.2 Rework the checkout summary page, its loading skeleton and error state; verify `src/app/(checkout)/checkout/(summary)/page.test.tsx` passes
- [x] 1.3 Rework the pay page, its loading skeleton, not-found state and `PaymentSimulator`; verify the pay page and simulator tests pass

## 2. Verification

- [x] 2.1 `npx tsc --noEmit`, `npm run check` and `npm run build` in `team-frontend`
- [x] 2.2 Load the checkout routes at 375, 768 and 1280 px on `next start`: no horizontal overflow and interactive controls at least 44 px high
- [x] 2.3 `openspec validate team-frontend-layout-checkout --strict`
