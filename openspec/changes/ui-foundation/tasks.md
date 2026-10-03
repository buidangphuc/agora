## 1. Code — team-frontend: green gate

- [ ] 1.1 Run `npx biome check --write .` (safe fixes only) and fix the remaining Biome errors by hand (Modal a11y included); verify `npx biome check .` exits 0
- [ ] 1.2 Fix the 11 `tsc` errors (CheckoutView `orderId`/`price`, RecommendationsRow `heading`, SearchImpressions `itemListId`, track.test `buildBeacon`, ...) against the real types, no `any`/`@ts-ignore`; verify `npx tsc --noEmit` exits 0
- [ ] 1.3 Fix the 5 failing Vitest tests; verify `npx vitest run` is all green
- [ ] 1.4 Commit the green baseline (WIP primitives + fixes); verify `npm run check` and `npx next build` pass

## 2. Code — team-frontend: tokens

- [ ] 2.1 Complete Tier 1 in `tailwind.config.ts` (neutral, accents, type scale 12/14/16/20/24, radius, shadows); verify `npx next build` passes
- [ ] 2.2 Add Tier 2 CSS variables to `src/app/globals.css` and map them as Tailwind colours; verify a unit test renders a `bg-action-primary` element with the brand colour
- [ ] 2.3 Add `scripts/check-tokens.mjs` with `tokens-allow` support and a Vitest test of its matcher on good and bad samples; verify the test passes
- [ ] 2.4 Append the token lint to the `check` script; verify it currently reports the known violations
- [ ] 2.5 Replace all raw hex and arbitrary values with tokens, one commit per feature folder; verify the token lint reports 0 and `npm run check` is green
- [ ] 2.6 Update `UI_SYSTEM_DESIGN.md` §2 with the alias table and the Ant Design mapping; verify links resolve

- [ ] 2.7 Add `src/lib/action-result.ts` (`ActionResult<T>`, `ok`, `fail`) with a type-narrowing Vitest test; verify `npx vitest run src/lib` and `npx tsc --noEmit` pass

## 3. Code — team-frontend: Exception and loading shells

- [ ] 3.1 Add `src/app/not-found.tsx` (404 result + link home); verify `curl -s -o /dev/null -w '%{http_code}' :3000/does-not-exist` prints 404
- [ ] 3.2 Add `src/app/error.tsx` (client, retry via `reset()`) and `src/app/loading.tsx` (shell skeleton); verify a Vitest test renders both

## 4. E2E — platform-e2e

- [ ] 4.1 Add `team-frontend/FEATURES.yaml` entries for every scenario in this change (`status: planned`); verify `make -C platform-e2e features-check`
- [ ] 4.2 Add `tests/e2e/features/frontend/ui_foundation.feature` + steps: 404 page, computed brand colour, alias override; verify the scenarios pass and flip to `automated`
- [ ] 4.3 Run `openspec validate ui-foundation --strict`; verify it is valid
