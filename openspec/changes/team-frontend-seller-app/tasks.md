## 1. Seller app

- [x] 1.1 Create `team-frontend-seller` from the `team-frontend` toolchain (port 3001, Dockerfile with vendor copy, proto, README); verify `npm ci` and `npm run proto`
- [x] 1.2 Move seller routes, features and seller-only listing/order pieces with their tests; prune unreachable copies; verify `tsc` has no errors
- [x] 1.3 Seller session via `@fe-platform/session` (`seller_*` cookies, middleware, login, `/forbidden`) and `buyerAppUrl()` links; verify the session, middleware, layout and forbidden tests pass

## 2. Buyer app

- [x] 2.1 Delete seller routes and features, keep modules the buyer and admin code still import, point header links at `sellerAppUrl()`; verify `src/app/sellerRoutes.test.ts` and `npm run check`

## 3. Platform and infra

- [x] 3.1 `vendor.targets` for all apps, skipping missing ones; verify `fe-platform` `npm run check`
- [x] 3.2 Compose service, CORS origins, AGENTS.md, CI workflow, repo doctor; verify `python3 scripts/repo_doctor.py` reports no frontend drift

## 4. E2E

- [x] 4.1 `SELLER_BASE_URL`, seller page objects and seller steps on the seller app; verify the touched Python modules import and ruff has no new findings

## 5. Verification

- [x] 5.1 `npm run check`, `npm run build` and `docker build` in `team-frontend-seller`
- [x] 5.2 `npm run check` and `npm run build` in `team-frontend`
- [x] 5.3 `openspec validate team-frontend-seller-app --strict`
