## 1. App bug (team-frontend, test-first)

- [x] 1.1 Failing vitest in `src/app/(shop)/notifications/page.test.tsx`: a `/seller/orders/{id}` `link_url` renders as `http://localhost:3001/seller/orders/{id}`; `/checkout/pay/{id}`, `/sellers/x` and an empty link stay `/checkout/pay/{id}`, `/sellers/x`, `#`; verify red
- [x] 1.2 `NotificationsView.tsx` routes `/seller` links through `sellerAppUrl()`; verify the test green and `npm run check` (biome, tsc, token lint, vitest)
- [x] 1.3 Rebuild `team-frontend:local` and recreate `team-frontend-svc` in `mainx`

## 2. Test drift (platform-e2e)

- [x] 2.1 Pattern A: `goto_streamed` in `src/core/base_page.py`, `BasePage.goto` uses it, `LoginPage.login` waits for `wait_streamed`; step helpers listed in `design.md` call `goto_streamed`; README "Streamed pages" updated
- [x] 2.2 Pattern B: semantic token classes `text-text-warning|success|danger` in `d1_account_steps.py` and `d2_orders_steps.py`
- [x] 2.3 Pattern C: `support/apps.py` routes `/admin*` to `ADMIN_BASE_URL`; login-return, referrer-hygiene and admin-cases steps use it; seller token lint runs in `team-frontend-seller`
- [x] 2.4 Pattern D and E: copy, markup and wait fixes listed in `design.md`
- [x] 2.5 Pin the gaps-document base of `promotion_admin_vouchers_steps.py` to `05978392`; grep for other `origin/main` diffs (none)
- [x] 2.6 ruff and black clean on every touched file; `make -C platform-e2e features-check`

## 3. Specs

- [x] 3.1 MODIFIED `ui-orders` responsive rule (1024px), `admin-moderation-ui` entry links, `order-lifecycle-ui` notification links
- [x] 3.2 `openspec validate e2e-ui-rebaseline --strict` and `scripts/asdlc/spec_lint.py e2e-ui-rebaseline`

## 4. Verify

- [x] 4.1 Re-run every file with a TEST-DRIFT or APP-BUG test, one file at a time, and record before and after in `design.md`
