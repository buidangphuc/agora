## Why

The admin (E1) and seller (E2) apps moved their screens out of `team-frontend`, but the buyer app still carried the gateway functions, helpers and tests that only those screens used. Plan P10 steps E3 and E4 finish the split: the buyer app keeps only buyer code, and moving between the three apps follows decision D3 (one session per app, no shared cookie, SSO hand-off later).

## What Changes

- E3, buyer slim-down (`team-frontend`): remove the seller- and admin-only exports left in `src/lib/gateway` (listing create/update/delete/upload, storefront upsert, bundle create, seller orders and status updates, return status update, seller wallet, plans/subscribe/entitlements, ad campaigns, magic listing, dispute list/get/resolve, report list/resolve, the `audit` and `accountAdmin` clients), the admin dispute action helpers, the header-based login guard, `useDialog` and their tests; move `usePending` out of `features/seller` into `src/lib`. `src/middleware.ts` already handles only the buyer session, so it is unchanged.
- E4, cross-app navigation: the buyer "Kênh Người Bán" links open the seller app at `/login?next=/seller` (`sellerEntryUrl()`); the seller and admin login pages read `next` (or the older `returnUrl`) through `safeReturnUrl` and send a user who already has that app's session straight to it; the seller app gains a "Đăng xuất" control (sidebar, mobile drawer, `/forbidden`); vitest proves each app's logout clears only its own cookies.
- Gap G8 (SSO hand-off needs a new identity RPC) is spelled out in `fe-platform/docs/gaps.md`.
- `platform-gitops`: baselines and staging/prod overlays (`COOKIE_SECURE: "true"`, own ingress host) for `team-frontend-seller` and `team-frontend-admin`, and `tests/test_frontend_cookie_secure_manifests.py` runs the same checks for all three apps.
- Docs: `team-frontend`, seller and admin READMEs, `AGENTS.md` (diagram, repo map, ports), `docs/agents/project.md` (feedback loops and checks for the three apps), `platform-gitops/README.md` known gaps; verification references that pointed at removed buyer tests now point at the admin copies.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (gateway modules and tests, `lib/apps.ts`, `ConsumerShell`), `team-frontend/FEATURES.yaml`, `team-frontend-seller/src` (login, forbidden, sidebar, drawer, logout button, nav icon), `team-frontend-admin/src` (login page, logout test), `platform-gitops/envs`, `platform-gitops/tests`, `fe-platform/docs/gaps.md`, `openspec/specs/dispute-cases-ui/spec.md`, docs listed above.
- Visible changes: the buyer seller-channel links go to the seller login, which forwards a signed-in seller to `/seller`; the seller app shows "Đăng xuất".

## Non-goals

SSO hand-off (G8, backend); wiring the seller and admin apps into Argo CD ApplicationSets and image tags; moving `VoucherManager` or the PDP review moderation controls out of the buyer app; trimming the vendored protos; any change to the admin voucher or campaign screens.

## Non-functional requirements

`npm run check` and `npm run build` green in all three apps; the gitops manifest tests pass; no `data-testid`, role or accessible name used by e2e changes.
