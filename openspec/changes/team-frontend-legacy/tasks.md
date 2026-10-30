## 1. Config

- [x] 1.1 Remove the legacy colour groups, `gray` alias, shopee shadows and the unused CSS rule; fix the missed `border-t-gray-900`; verify `tsc`, biome and a grep of non-test source for `gray-N`, `brand`, `shopee` are clean

## 2. Tests

- [x] 2.1 Update `tokens.test.ts` and `palette.test.ts`; verify green

## 3. Verification

- [x] 3.1 `npm run check` and `npm run build`; verify the built CSS has no `gray`, `brand` or `shopee` utility
- [x] 3.2 Run the B1 e2e colour scenarios against the local build (11 passed)

## 4. Done-condition

- [x] 4.1 `openspec validate team-frontend-legacy --strict`
