# team-frontend-seller

Next.js 15 (App Router, SSR, `output: "standalone"`) seller app of the marketplace: the seller centre that used to live under `/seller` in `team-frontend`. It is a UI shell like the buyer app: it owns no business capability, no database and no broker topics, and talks only to `team-gateway`. Decision: `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md` (three apps on the platform, ports 3000/3001/3002, per-app sessions).

Status: compose service `team-frontend-seller`, port `:3001`. Stack: React 19, Tailwind 3, Connect-ES (`@connectrpc/connect-node`), `@fe-platform/*` packages from `vendor/`, Biome, Vitest.

## 1. Contract

**Serves** (HTTP only):

| Route | What | Gate |
|---|---|---|
| `/` | Redirects to `/seller` | none |
| `/login` | Seller login form (same `LoginForm` and server action as the buyer app). Takes `?next=<path>` (the buyer app's "Kênh Người Bán" link sends `next=/seller`) or the older `?returnUrl=`, both checked by `safeReturnUrl`; a user who already has a seller session with `listing.write` is sent straight to it (default `/seller`) | none |
| `/sell` | Redirects to `/seller/new` | none |
| `GET`/`HEAD`/`POST /auth/handoff` | SSO hand-off landing (`src/app/auth/handoff/route.ts`, `createHandoffHandler` from `@fe-platform/session`): `GET ?code=&next=` peeks the code (`PeekHandoffCode`, target `SELLER`) and renders a confirmation form; `POST` (same-origin only) redeems it (`RedeemHandoffCode`), sets `seller_session`/`seller_refresh` and redirects to `next`. Any failure redirects to `/login?next=`. The middleware adds `no-store`, a `default-src 'none'` CSP and `X-Frame-Options: DENY`; the access log writes `code=REDACTED` (`src/lib/access-log.ts`) | Public; the code is the credential |
| `/seller/*` | Workplace (a Suspense-wrapped "Bắt đầu bán hàng" getting-started card above the KPI row, built from `GetStorefront`, `ListMyListings`, `ListMyShopVouchers` and `ListMyCampaigns` at page size 1 for the session's own seller id, 3 s per source, "chưa rõ" for an unavailable one, hidden once all four steps are done; the product list pages on the server through `ListMyListings` with `?status=&q=&cursor=` at 20 rows, the KPIs "Tổng sản phẩm"/"Đang bán" come from page `total` at page size 1 and recent orders from one `ListSellerOrders` call of 5 rows), `new`, `[id]/edit`, `orders`, `orders/print`, `orders/[id]`, `analytics`, `ads` (create form plus a campaign table paged at 20 with pause and resume; a list failure shows an error with a retry link and keeps the form), `marketing`, `bundles`, `plans` (price per term, period end and renewal state from `getMySubscription`; a read failure shows an error and disables every plan button), `shop`, `wallet`, `verification` (KYC upload) | `src/app/seller/layout.tsx`: `/login?returnUrl=` when anonymous; without `listing.write` it renders `SellerAccessDenied` in place ("Tài khoản Người Bán đang bị hạn chế") when the token's `status` is `restricted`, otherwise the `SellerOnboarding` terms screen ("Đồng ý và trở thành Người Bán" calls `AuthService.BecomeSeller` with the terms version, stores the returned access token and opens `/seller/new`; `src/features/seller/onboarding.ts`; the screen links to the buyer `/dieu-khoan-nguoi-ban` page through `buyerAppUrl()`, and after `BecomeSeller` succeeds the action calls `PrivacyService.RecordConsent` with purpose `SELLER_TERMS` and the terms version, best-effort: a consent write failure does not undo seller status, and a denied `BecomeSeller` writes no consent row), each with a link back to the buyer app (`BuyerStorefrontLink`: a hand-off to the buyer `/auth/handoff`, falling back to the buyer `/login?next=/`) and a "Đăng xuất" button; the gateway re-enforces scopes on the data. `/seller/orders` selects up to 50 to-ship orders for bulk shipping (`BulkShipModal`, `bulkShipOrdersAction` in `src/features/order/actions.ts`: one `UpdateOrderStatus` SHIPPED per distinct order with an optional tracking number, 4 at a time within 15 s, failed rows stay selected) and `/seller/orders/print?ids=` renders packing slips for up to 50 of the seller's own orders, listing the ids it could not print (`src/app/seller/orders/print/page.tsx`). `/seller/orders` pages on the server: one `ListSellerOrders` call (`listSellerOrdersPage`, `src/lib/gateway/orders.ts`) per view with `?status=&q=&cursor=&page=` at 20 rows, tab badges from the response `status_counts`, `q` cut to 100 characters, and an unknown cursor or a failed call shows a retry state. `/seller/ads` and `/seller/bundles` pick listings through `ListingPicker` (`src/features/seller/ListingPicker.tsx`), which searches the seller's published listings via `searchMyListingsAction` (20 options, `listing.write` required). `/seller/marketing` is the marketing hub (tabs `?tab=voucher|flashsale`, cursor paging `?cursor=`, flash-sale `?status=`): the seller's own shop vouchers and flash-sale campaigns with create, deactivate and cancel, all behind `listing.write`; the seller id never travels in a request. `/seller/shop` edits the shop name, banner (10 MB JPEG/PNG/WebP), tagline (120 characters), up to 6 featured listings and the theme, pre-filled from `GetStorefront` with featured titles resolved through `getListing`; a save sends the full storefront through `upsertStorefrontAction` |

There is no `/admin` route and no link to the admin app.

Every page is `noindex, nofollow` and titled `<page> | Agora` (`src/app/layout.tsx` metadata `robots` and title template).

**Consumes**: only `team-gateway` (`GATEWAY_URL`) through `src/lib/gateway/*` (`server-only`, copied from `team-frontend`). Links to buyer pages (public listing, public shop, chat, home) go to the buyer app through `buyerAppUrl()` in `src/lib/apps.ts`.

**Session** (`@fe-platform/session`, preset `seller`, `src/lib/session-config.ts`): cookies `seller_session` (access, 1 hour) and `seller_refresh` (refresh, 30 days), `httpOnly`, `sameSite=lax`, `path=/`, no `Domain`. On HTTPS (`X-Forwarded-Proto: https` or `COOKIE_SECURE=true`) both names carry the `__Host-` prefix and the `Secure` attribute. `src/middleware.ts` is `createSessionMiddleware` from the package: it refreshes an unusable access cookie once through the gateway, clears both cookies on `Unauthenticated`/`PermissionDenied`, and renders anonymous on a transient failure. The buyer app keeps `session`/`agora_refresh`, so the two apps never overwrite each other's cookies on the same host.

## 2. Events

None. Seller routes carry no tracking components (`src/app/seller/tracking.test.ts`). `trackAiInteraction` in the listing studio pushes to `window.dataLayer` only; no GTM container loads in this app.

## 3. Data

No database, no migrations.

## 4. Configuration

| Var | Default | Used in |
|---|---|---|
| `GATEWAY_URL` | `http://127.0.0.1:8080` | `src/lib/gateway/config.ts` |
| `NEXT_PUBLIC_GATEWAY_URL` | `http://localhost:8080` (build-time) | `next.config.mjs` (CSP) |
| `NEXT_PUBLIC_BUYER_APP_URL` | `http://localhost:3000` (build-time, Dockerfile `ARG`); every link to the buyer app goes through `buyerAppUrl()` | `src/lib/apps.ts` |
| `NEXT_PUBLIC_MEDIA_BASE_URL` | `http://localhost:9000/listing-images` (build-time) | `src/lib/media.ts` |
| `OTEL_SERVICE_NAME` | `team-frontend` | `src/lib/gateway/config.ts` |
| `TRUSTED_PROXY_HOPS` | `1` | `src/lib/gateway/client-context.ts` |
| `COOKIE_SECURE` | unset (`true` forces `Secure` and the `__Host-` prefix; TLS environments only) | `@fe-platform/session` via `src/lib/gateway/session-cookie.ts` |
| `NEXT_PUBLIC_SELLER_APP_URL` | `http://localhost:3001` (build-time); this app's own origin, checked against the `Origin` of a hand-off `POST` | `src/features/handoff/config.ts` |
| `NEXT_PUBLIC_SELLER_TERMS_VERSION` | `2026-11-02` (build-time); the terms version sent to `BecomeSeller`, must equal the first identity `SELLER_TERMS_VERSIONS` entry | `src/features/seller/onboardingTerms.ts` |

`NEXT_PUBLIC_*` values are baked in at `next build`; the Dockerfile takes them as build args.

## 5. Run locally

```bash
docker compose -f docker-compose.services.yaml up --build team-frontend-seller team-gateway   # plus the services you need
```

Standalone dev (needs a gateway on `GATEWAY_URL`):

```bash
cd team-frontend-seller
npm ci
npm run proto        # buf generate into src/generated (gitignored)
npm run dev          # http://localhost:3001
```

## 6. Build, test and lint

| Command | Does |
|---|---|
| `npm run proto` | `buf generate` from the vendored `proto/` into `src/generated/` |
| `npm run dev` / `build` / `start` | Next dev server on 3001 / production build / serve on 3001 |
| `npm test` | `vitest run` |
| `npm run check` | PR gate: `biome check .` + `tsc --noEmit` + `node scripts/check-tokens.mjs` + `vitest run` |

CI: `.github/workflows/team-frontend-seller-ci.yml` (check, build, `docker build` from this directory alone).

## 7. Spec and verification

- OpenSpec change `openspec/changes/archive/2026-11-02-team-frontend-seller-app`.
- E2E: `platform-e2e` drives this app at `SELLER_BASE_URL` (default `http://localhost:3001`); seller page objects set `app = "seller"` and `tests/e2e/support/apps.py` routes `/seller*` and `/sell` here.
- Feature manifest: `FEATURES.yaml` in this directory (250 entries, `grep -c '^  - id:' FEATURES.yaml`: those moved from `team-frontend/FEATURES.yaml`, every entry routed under `/seller` or `/sell` or verified by a test in this app, plus the SSO hand-off and seller onboarding entries). Gate: `make -C platform-e2e features-check`.

## 8. Shared platform packages

`@fe-platform/tokens`, `ui`, `api-client` and `session` are committed tarballs in `vendor/`. `npm run vendor` in `fe-platform/` writes them for every app listed in `vendor.targets`; then run `npm install` here, check the `integrity` of the four `@fe-platform/*` entries in `package-lock.json`, and commit `vendor/` with the lock file. `vendor:check` in `fe-platform` fails when a tarball here is stale.

## 9. Known gaps

- Duplicated from `team-frontend` on purpose (to be shared or trimmed later): `src/lib/gateway/*` (gateway clients, refresh, revoke, token expiry), `src/components/ui/*` facades, `src/features/auth/*` (login form, actions, return URL), `src/features/order/OrderStatusBadge.tsx`, `src/features/account/skeletons.tsx`, `src/lib/analytics/*` (only for `trackAiInteraction`), `src/lib/action-result.ts`, `src/lib/requestCache.ts`, `src/lib/media.ts`, `src/lib/useRouteRetry.ts`, `scripts/check-tokens.mjs`, `scripts/patch-react-dom-ping.mjs`, `tailwind.config.ts`, `src/app/globals.css` and the vendored `proto/`.
- SSO with the buyer app is a one-time hand-off code (buyer "Kênh Người Bán" and the seller layout's "Về trang mua sắm"); other links to the buyer app, such as "Chăm sóc khách hàng", still open it with whatever buyer session it has. Logout ("Đăng xuất" in the sidebar, the mobile drawer and the in-place 403) revokes the server session and clears only `seller_session`/`seller_refresh`.
- `NEXT_PUBLIC_SELLER_APP_URL` and `NEXT_PUBLIC_SELLER_TERMS_VERSION` are not in `.env.example` and not Dockerfile build args, so the image bakes `http://localhost:3001` as its own origin (a hand-off `POST` from any other host is refused) and `2026-11-02` as the terms version.
- Locally the gateway trusts this app: compose sets `TRUSTED_PROXIES=team-frontend-svc,team-frontend-seller-svc,team-frontend-admin-svc` (`docker-compose.services.yaml:580`). The gitops gateway config still trusts only `team-frontend` (`platform-gitops/envs/services/team-gateway.yaml:52`, `platform-gitops/argocd/apps/team-gateway.yaml:71`), so in a deployed environment the gateway ignores the `X-Forwarded-For` this app sends on login and refresh and records the pod IP instead of the browser IP.
- Compose passes `NEXT_PUBLIC_BUYER_APP_URL` as a runtime variable, which has no effect on the built image; the image bakes the `http://localhost:3000` default. Other hosts need `--build-arg NEXT_PUBLIC_BUYER_APP_URL=...`.
- `OTEL_SERVICE_NAME` defaults to `team-frontend` in `src/lib/gateway/config.ts`; set it to `team-frontend-seller` in deployed environments.
- The gitops baseline and staging/prod overlays (`platform-gitops/envs/{services,staging/services,prod/services}/team-frontend-seller.yaml`, `COOKIE_SECURE: "true"`) exist but are not wired into any ApplicationSet or image tag yet (`platform-gitops/README.md` known gaps).
