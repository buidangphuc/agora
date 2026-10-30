## Decisions

- Routes keep their paths: the seller app serves `/seller/*` and `/sell`, and `/` redirects to `/seller`. E2E page objects only change their base URL, and in-app links (`/seller/orders`, `/seller/new`) stay relative.
- What moved was found by reachability: the seller app started as a copy of `team-frontend`, every module not reachable from its routes or middleware was removed, and tests whose subject was removed went with it. In `team-frontend`, modules that became unreachable after the deletion were removed (`features/listing/validation.ts`, `reasonLabels.ts`, `lib/gateway/analytics.ts`).
- Shared modules are copied, not extracted to `fe-platform`: only code used by two or more apps and worth sharing goes to the platform (ADR-0006), and that decision waits until the admin app exists.
- `features/seller/LinkButton.tsx` and `usePending.ts` stay in `team-frontend` because the admin pages (removed by the admin app change) and `ReportListingButton` import them; plan step E3 cleans them up.
- Session: the seller app reads and writes cookies only through the `seller` preset. `getToken()` derives the cookie name from the request (`__Host-` on HTTPS), login and logout use the same names, and the middleware is the package's `createSessionMiddleware` with the app's own Connect refresh call and client context.
- No-scope users are redirected to `/forbidden` instead of seeing an inline result in the layout; the page keeps the copy that e2e asserts ("Cần tài khoản Người Bán" and the restricted notice). There is no seller onboarding in the buyer app, so the page links to `/sell` here.
- Cross-app links use one helper per app (`sellerAppUrl()` in the buyer app, `buyerAppUrl()` in the seller app) reading a `NEXT_PUBLIC_*` URL with a localhost default.
- `vendor.targets` already lists `../team-frontend-admin/vendor`; a target whose app directory is missing is skipped, so the admin app joins without another script change.

## Risks

- E2E was not run against a live stack. Seller scenarios that log in through the buyer login form and then open `/seller` now need a seller login on 3001; API logins set both cookies.
- Seller pages that relied on buyer chrome (header search, cart) lose it; the seller shell has its own sidebar and drawer.
- Duplicated gateway and UI code can drift between the two apps until it is shared or trimmed.
