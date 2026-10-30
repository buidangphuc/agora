## 1. Buyer slim-down (E3)

- [x] 1.1 Scan `team-frontend/src` for files and exports unused since the split (reachability from Next entry files, compared with base `585b958f`); verify the list against `tsc`
- [x] 1.2 Remove the seller/admin-only gateway functions, clients, helpers and their tests; move `usePending` to `src/lib`; verify `tsc --noEmit` and the gateway, dispute and auth vitest suites are green
- [x] 1.3 Point verification references for removed buyer tests at the admin copies (`FEATURES.yaml`, `openspec/specs/dispute-cases-ui`); verify every referenced test name exists

## 2. Cross-app navigation (E4)

- [x] 2.1 `sellerEntryUrl()` and the buyer shell links with `next`; verify `apps.test.ts` and `ConsumerShell.test.tsx` (red first, then green)
- [x] 2.2 Seller and admin login pages read `next`, redirect a signed-in user with access; verify the login page tests (red first, then green)
- [x] 2.3 Seller logout control in sidebar, drawer and `/forbidden`; verify `SellerShell.test.tsx` and the forbidden page test (red first, then green)
- [x] 2.4 Per-app logout tests in buyer, seller and admin; verify each deletes only its own cookie pair
- [x] 2.5 Record gap G8 in `fe-platform/docs/gaps.md`

## 3. Gitops

- [x] 3.1 Seller and admin baselines and staging/prod `COOKIE_SECURE` overlays; extend `test_frontend_cookie_secure_manifests.py` to all three apps; verify it red before the manifests and green after, and the whole `platform-gitops/tests` suite

## 4. Docs and verification

- [x] 4.1 Update the three app READMEs, `AGENTS.md`, `docs/agents/project.md`, `platform-gitops/README.md`
- [x] 4.2 `npm run check` and `npm run build` in `team-frontend`, `team-frontend-seller` and `team-frontend-admin`
- [x] 4.3 `openspec validate team-frontend-buyer-slim --strict`
