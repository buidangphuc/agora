# team-frontend-seller

Next.js 15 (App Router, SSR, `output: "standalone"`) seller app of the marketplace: the seller centre that used to live under `/seller` in `team-frontend`. It is a UI shell like the buyer app: it owns no business capability, no database and no broker topics, and talks only to `team-gateway`. Decision: `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md` (three apps on the platform, ports 3000/3001/3002, per-app sessions).

Status: compose service `team-frontend-seller`, port `:3001`. Stack: React 19, Tailwind 3, Connect-ES (`@connectrpc/connect-node`), `@fe-platform/*` packages from `vendor/`, Biome, Vitest.

## 1. Contract

**Serves** (HTTP only):

| Route | What | Gate |
|---|---|---|
| `/` | Redirects to `/seller` | none |
| `/login` | Seller login form (same `LoginForm` and server action as the buyer app) | none |
| `/forbidden` | "Không có quyền" page for a signed-in user without scope `listing.write`; links to `/sell` and back to the buyer app | none |
| `/sell` | Redirects to `/seller/new` | none |
| `/seller/*` | Workplace, `new`, `[id]/edit`, `orders`, `orders/[id]`, `analytics`, `ads`, `bundles`, `plans`, `shop`, `wallet` | `src/app/seller/layout.tsx`: `/login?returnUrl=` when anonymous, `/forbidden` without `listing.write`; the gateway re-enforces scopes on the data |

There is no `/admin` route and no link to the admin app.

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
| `NEXT_PUBLIC_BUYER_APP_URL` | `http://localhost:3000` (build-time) | `src/lib/apps.ts` |
| `NEXT_PUBLIC_MEDIA_BASE_URL` | `http://localhost:9000/listing-images` (build-time) | `src/lib/media.ts` |
| `OTEL_SERVICE_NAME` | `team-frontend` | `src/lib/gateway/config.ts` |
| `TRUSTED_PROXY_HOPS` | `1` | `src/lib/gateway/client-context.ts` |
| `COOKIE_SECURE` | unset (`true` forces `Secure` and the `__Host-` prefix; TLS environments only) | `@fe-platform/session` via `src/lib/gateway/session-cookie.ts` |

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

- OpenSpec change `openspec/changes/team-frontend-seller-app`.
- E2E: `platform-e2e` drives this app at `SELLER_BASE_URL` (default `http://localhost:3001`); seller page objects set `app = "seller"` and `tests/e2e/support/apps.py` routes `/seller*` and `/sell` here. The seller features are still listed in `team-frontend/FEATURES.yaml`.

## 8. Shared platform packages

`@fe-platform/tokens`, `ui`, `api-client` and `session` are committed tarballs in `vendor/`. `npm run vendor` in `fe-platform/` writes them for every app listed in `vendor.targets`; then run `npm install` here, check the `integrity` of the four `@fe-platform/*` entries in `package-lock.json`, and commit `vendor/` with the lock file. `vendor:check` in `fe-platform` fails when a tarball here is stale.

## 9. Known gaps

- Duplicated from `team-frontend` on purpose (to be shared or trimmed later): `src/lib/gateway/*` (gateway clients, refresh, revoke, token expiry), `src/components/ui/*` facades, `src/features/auth/*` (login form, actions, return URL), `src/features/order/OrderStatusBadge.tsx`, `src/features/account/skeletons.tsx`, `src/lib/analytics/*` (only for `trackAiInteraction`), `src/lib/action-result.ts`, `src/lib/requestCache.ts`, `src/lib/media.ts`, `src/lib/useRouteRetry.ts`, `scripts/check-tokens.mjs`, `scripts/patch-react-dom-ping.mjs`, `tailwind.config.ts`, `src/app/globals.css` and the vendored `proto/`.
- No SSO between apps: a buyer signed in on `:3000` signs in again here, and the "Chăm sóc khách hàng" link opens the buyer chat with the buyer session (gap G8 in ADR-0006).
- `/sell` still redirects to `/seller/new`; there is no seller onboarding flow, so the forbidden page links there.
- `TRUSTED_PROXIES` in the gateway lists only `team-frontend-svc`, so the client IP this app forwards on login and refresh is ignored by the gateway.
- `OTEL_SERVICE_NAME` defaults to `team-frontend` in `src/lib/gateway/config.ts`; set it to `team-frontend-seller` in deployed environments.
- No gitops overlay or `COOKIE_SECURE` manifest for this app yet (plan step E4).
