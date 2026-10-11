## 1. Code — team-frontend: green gate

- [x] 1.1 Run `npx biome check --write .` (safe fixes only) and fix the remaining Biome errors by hand (Modal a11y included); verify `npx biome check .` exits 0
- [x] 1.2 Fix the 11 `tsc` errors (CheckoutView `orderId`/`price`, RecommendationsRow `heading`, SearchImpressions `itemListId`, track.test `buildBeacon`, ...) against the real types, no `any`/`@ts-ignore`; verify `npx tsc --noEmit` exits 0
- [x] 1.3 Fix the 5 failing Vitest tests; verify `npx vitest run` is all green
- [x] 1.4 Commit the green baseline (WIP primitives + fixes); verify `npm run check` and `npx next build` pass

## 2. Code — team-frontend: tokens

- [x] 2.1 Complete Tier 1 in `tailwind.config.ts` (neutral, accents, type scale 12/14/16/20/24, radius, shadows); verify `npx next build` passes
- [x] 2.2 Add Tier 2 CSS variables to `src/app/globals.css` and map them as Tailwind colours; verify a unit test renders a `bg-action-primary` element with the brand colour
- [x] 2.3 Add `scripts/check-tokens.mjs` with `tokens-allow` support and a Vitest test of its matcher on good and bad samples; verify the test passes
- [x] 2.4 Append the token lint to the `check` script; verify it currently reports the known violations
- [x] 2.5 Replace all raw hex and arbitrary values with tokens, one commit per feature folder; verify the token lint reports 0 and `npm run check` is green
- [x] 2.6 Update `UI_SYSTEM_DESIGN.md` §2 with the alias table and the Ant Design mapping; verify links resolve

- [x] 2.7 Add `src/lib/action-result.ts` (`ActionResult<T>`, `ok`, `fail`) with a type-narrowing Vitest test; verify `npx vitest run src/lib` and `npx tsc --noEmit` pass

## 3. Code — team-frontend: Exception and loading shells

- [x] 3.1 Add `src/app/not-found.tsx` (404 result + link home); verify `curl -s -o /dev/null -w '%{http_code}' :3000/does-not-exist` prints 404
- [x] 3.2 Add `src/app/error.tsx` (client, retry via `reset()`) and `src/app/loading.tsx` (shell skeleton); verify a Vitest test renders both

## 4. E2E — platform-e2e

- [x] 4.1 Add `team-frontend/FEATURES.yaml` entries for every scenario in this change (`status: planned`); verify `make -C platform-e2e features-check`
- [x] 4.2 Add `tests/e2e/features/frontend/ui_foundation.feature` + steps: 404 page, computed brand colour, alias override; verify the scenarios pass and flip to `automated` (Ran green against the agora stack (production frontend): 404 page, computed brand colour, alias override, plus the `/dev/ui` 404 scenario; entries flipped to `automated`.)
  - Blocked: authored and collected (`pytest --collect-only`: 3 scenarios). The 404 and brand-colour scenarios passed against a local `next start` (no backend); the alias-override scenario needs a rendered listing (PriceTag), which needs the agora stack (gateway + search + seed), and the stack is not running. FEATURES.yaml entries stay `planned` until it has run green.
- [x] 4.3 Run `openspec validate ui-foundation --strict`; verify it is valid

## Evidence (2026-10-09)

- Gate on feat/ui-system after the UI fixes and a frontend rebuild:
  - parallel lane: 714 passed, run twice (w3-par-1, w3-par-2);
  - destructive lane: 97 passed (w3-destr);
  - the stack was READY afterwards.
- spec_sync --strict reports e2e-ready. Unit-verified scenarios carry a VERIFIED BY line and a not-testable FEATURES
  entry (platform-e2e README, spec-check).
- Defects found by e2e and fixed in this wave:
  - voucher layout shift;
  - checkout Stepper shown under the kill-switch;
  - PDP footer and chat bubble under the buy bar;
  - category pills missing on /search;
  - card-title attribution;
  - review paging;
  - route-error retry that did not re-fetch;
  - outages shown as empty (cart, sessions, notifications);
  - home page crash when team-domain is down;
  - orders retry served from the router cache.
