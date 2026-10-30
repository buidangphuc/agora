# team-frontend-admin

Next.js 15 (App Router, SSR, `output: "standalone"`) admin app of the marketplace, on its own host (ADR-0006, `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md`). It renders the operator screens and holds the admin session cookie. It owns no business capability, no database and no topics; the gateway enforces every scope on the data.

Status: compose service `team-frontend-admin`, port `:3002`. Stack: React 19, Tailwind 3, Connect-ES (`@connectrpc/connect-node`), `@fe-platform/{tokens,ui,api-client,session}` from vendored tarballs, Biome, Vitest.

## 1. Contract

**Serves** (HTTP only):

| Surface | What | Authorization |
|---|---|---|
| `/` | Redirects to `/admin/cockpit` | none |
| `/login` | Admin sign-in (no registration link, no sign-up route). Takes `?next=<path>` or `?returnUrl=`, both checked by `safeReturnUrl`; a session that already has scope `admin` goes straight there (default `/admin/cockpit`) | public |
| `/admin/cockpit`, `/admin/listings`, `/admin/cases`, `/admin/users`, `/admin/audit`, `/admin/vouchers`, `/admin/vouchers/new`, `/admin/vouchers/[code]` | Ops cockpit, listing moderation, reports and disputes, user administration, audit log, platform vouchers (list, create, detail) | `src/app/admin/layout.tsx`: `/login?returnUrl=` without a session, "Không có quyền" unless scope `admin`; each page and action re-checks, the gateway re-enforces |
| `/admin/campaigns` | Active flash-sale campaigns (`FlashSaleService.ListActiveCampaigns`, paged by `?cursor=`) and a lookup of one listing's active sale (`GetActiveFlashSale`, `?listing=`) | same as above; the promotion service lets any caller read both |
| `/admin/campaigns/new` | Create a flash sale (`FlashSaleService.CreateCampaign`): form, review dialog with a summary, then the campaign the server returned | same as above; the promotion service requires `admin` or `listing.write` and audits an admin create (fail-closed) |
| `/admin/plans` | Seller subscription plans, read only (`SubscriptionService.ListPlans`) | same as above; the promotion service serves the catalogue to any caller |
| `/forbidden` | The "Không có quyền" page with a logout button | none |
| `GET /api/admin/metrics` | Cockpit refresh, proxies `fetchCockpit` | 401 without a session; the gateway enforces scope `admin` |

Isolation: `public/robots.txt` disallows everything; `next.config.mjs` sends `X-Robots-Tag: noindex, nofollow`, a CSP with `frame-ancestors 'none'` and `Referrer-Policy: strict-origin-when-cross-origin` on every route; every page also declares `referrer: no-referrer` (`src/app/referrerMetadata.test.ts`). The buyer app has no `/admin` route or link.

The screens keep the `/admin/*` paths on this host so `data-testid`s, roles, links between screens and the e2e page objects did not change.

**Consumes**: only `team-gateway` (`GATEWAY_URL`), through the clients in `src/lib/gateway/*` (`server-only`), built per request with the caller's token.

Platform vouchers (`src/lib/gateway/vouchers.ts`, `src/features/vouchers/`): `VoucherService.ListVouchers` with an empty `seller_id` (platform vouchers, page size 20, cursor in `?cursor=`), `GetVoucher` by code, `CreateVoucher` with scope `PLATFORM` (team-promotion requires scope `admin` and writes an audit event before inserting). The form validates on the client and again in the Server Action; creating needs a review dialog plus an acknowledgement checkbox, and the success view shows the voucher the service returned. Errors map by code to Vietnamese messages, never the backend text: `UNAVAILABLE` on create is shown as "audit or promotion down, nothing was created" (team-promotion fails closed when team-audit is down), `DEADLINE_EXCEEDED` as "outcome unknown, check the list".

Session (`@fe-platform/session`, preset `admin`): cookies `admin_session` (access, `httpOnly`, `sameSite=strict`, `maxAge=1800`) and `admin_refresh` (`maxAge=28800`), no `Domain`, `__Host-` prefix when the request is secure (`COOKIE_SECURE=true` or `X-Forwarded-Proto: https`). `src/middleware.ts` is `createSessionMiddleware` with the admin preset: it refreshes an unusable access cookie once through the refresh cookie and sets `x-pathname` for the login return URL. The buyer cookies `session`/`agora_refresh` are ignored here, so signing in or out of one app does not touch the other.

## 2. Events

None.

## 3. Run

```bash
npm ci --ignore-scripts
npm run proto          # Connect stubs into src/generated (gitignored)
npm run dev            # http://localhost:3002
npm run check          # biome + tsc + token lint + vitest
npm run build
docker build -t team-frontend-admin:local .
```

Shared packages come from `vendor/*.tgz`, written by `npm run vendor` in `fe-platform` (checked by `npm run vendor:check` there). After a revendor, rewrite the `integrity` of the four `@fe-platform/*` entries in `package-lock.json` from the new tarballs and run `npm ci`.

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_URL` | `http://127.0.0.1:8080` | Gateway base URL for server-side calls |
| `NEXT_PUBLIC_GATEWAY_URL` | `http://localhost:8080` | Gateway origin allowed in the CSP `connect-src` (build time) |
| `TRUSTED_PROXY_HOPS` | `1` | Reverse proxies in front of this server, for the client IP sent at login |
| `OTEL_SERVICE_NAME` | `team-frontend` | Service name on gateway calls (`src/lib/gateway/config.ts`) |
| `COOKIE_SECURE` | unset | `true` forces `Secure` and the `__Host-` prefix on the admin cookies (TLS environments only) |

There is no `NEXT_PUBLIC_*_APP_URL`: the admin app links to neither the buyer nor the seller app, and neither of them links here. `NEXT_PUBLIC_*` values are baked in at `next build`; the Dockerfile takes `NEXT_PUBLIC_GATEWAY_URL` and `NEXT_PUBLIC_APP_ENV` as build args.

Local stack (from the repo root; the gateway has no `depends_on` link, so start it too):

```bash
docker compose -f docker-compose.services.yaml up --build team-frontend-admin team-gateway   # plus the services the screen needs
```

CI: `.github/workflows/team-frontend-admin-ci.yml` (`npm run proto`, `check`, `build`, `docker build` from this directory alone).

## 4. Spec and verification

- OpenSpec changes `team-frontend-admin-app`, `team-frontend-admin-vouchers`, `team-frontend-admin-campaigns`, `team-frontend-layout-admin-ops`, `team-frontend-layout-admin-promo`.
- Feature manifest: `FEATURES.yaml` in this directory (211 entries moved from `team-frontend/FEATURES.yaml`: every entry routed under `/admin` or verified by a test in this app, plus the dispute-cases browser scenarios). Gate: `make -C platform-e2e features-check`.
- E2E: admin page objects set `base_url_setting = "admin_base_url"` (`ADMIN_BASE_URL`, default `http://localhost:3002`) and seed the `admin_session` cookie (`tests/e2e/flows/auth_flow.py`).

## 5. Known gaps

- Duplicated with `team-frontend` until shared pieces move to `fe-platform` (plan step D): `src/lib/gateway/` (client, auth interceptor, config, client context, refresh, revoke, token expiry, disputes, reports), `src/components/ui/` (facades over `@fe-platform/ui` plus local Badge, Descriptions, Empty, Modal, Table, Toast), `src/features/auth/` (login form, return URL, guard path), `src/features/dispute/` (resolve action, allowed actions), `src/lib/{action-result,useRouteRetry}.ts`, `src/components/LinkButton.tsx` and `src/lib/usePending.ts` (copied from the seller feature), `tailwind.config.ts`, `scripts/`, `proto/`.
- Platform vouchers cannot be edited, stopped or deleted: `VoucherService` has no RPC for it (gap G9 in `fe-platform/docs/gaps.md`). The confirm dialog says so.
- A duplicate voucher code is not reported as `ALREADY_EXISTS`: team-promotion returns an unmapped insert error (`INTERNAL` at the gateway), so the form shows a generic "code may already exist" message (gap G10).
- `UNAVAILABLE` on `CreateVoucher` cannot tell audit-down from promotion-down; the message names both (gap G11).
- Voucher screens were tested with a fake Connect transport only, not against a running team-promotion, team-audit and gateway; the buyer checkout redemption of a new platform voucher (plan E1c check) is not verified.
- Campaigns (`fe-platform/docs/gaps.md` G12-G14): there is no RPC to list every campaign (scheduled or ended) or to cancel or edit one, so `/admin/campaigns` shows only active campaigns and says so. `CreateCampaign` has no idempotency key, so after a timeout the screen tells the operator to check the active list instead of retrying. The admin create path does not check that the listing exists and the service does not validate price, stock cap or window; the form validates them (positive integers, end after start and in the future). The flash-sale kill-switch makes the active list empty with no signal, so "no campaigns" and "switched off" look the same. Audit-down and promotion-down both reach the app as `UNAVAILABLE` with a sanitized message, so the error names both.
- Campaign and plan screens are tested with an in-memory Connect router transport and in the browser against an unreachable gateway; they have not been run against the real promotion, audit and gateway services.
- Ingress is not limited to a VPN or IP allow-list (platform-gitops, ADR-0006 open risk). The baseline and staging/prod overlays exist (`platform-gitops/envs/{services,staging/services,prod/services}/team-frontend-admin.yaml`) but are not wired into any ApplicationSet or image tag yet (`platform-gitops/README.md` known gaps).
- Locally the gateway trusts this app (`TRUSTED_PROXIES` in `docker-compose.services.yaml:580`); the gitops gateway config trusts only `team-frontend` (`platform-gitops/envs/services/team-gateway.yaml:52`, `platform-gitops/argocd/apps/team-gateway.yaml:71`), so a deployed gateway records the pod IP instead of the operator's browser IP on admin login and refresh.
- `OTEL_SERVICE_NAME` defaults to `team-frontend`; set it to `team-frontend-admin` in deployed environments.
- No single sign-on (gap G8, ADR-0006): an operator signs in here even with a buyer or seller session, and admin stays excluded from any future hand-off.
- e2e: the admin scenarios now open `ADMIN_BASE_URL` (default `http://localhost:3002`) and seed the `admin_session` cookie, but they have not been run against this app; `platform-e2e/docker-compose.e2e.yaml` does not set `ADMIN_BASE_URL`, so the containerised runner falls back to `localhost:3002`; scenarios that expected GA4 pushes on admin pages no longer apply (this app ships no analytics).
