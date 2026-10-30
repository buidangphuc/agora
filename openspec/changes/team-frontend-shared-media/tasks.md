## 1. Platform

- [x] 1.1 Tokens (`priceTag`, `image`, `avatar`, `drawer`, spacing 14 and 20, preset aspect ratios) and `npm run build` in `packages/tokens`; verify the token tests pass
- [x] 1.2 `PriceTag`, `formatPrice`, `Image`, `Avatar`, `initials`, `Drawer` with JSDoc, states, typecheck cases, tests, catalog entries and a changeset; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Revendor, refresh the lock integrity, rewrite the four team-frontend components as facades and delete `ImageClient.tsx`; verify the component tests pass

## 3. Call sites

- [x] 3.1 Replace `className` on `Image` with `radius` and wrap the two `PriceTag` sites; verify `tsc` has no errors
- [x] 3.2 Update the Drawer and FilterSidebar tests for the Radix overlay, scroll lock and async focus return; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-shared-media --strict`
