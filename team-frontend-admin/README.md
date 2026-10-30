# team-frontend-admin

Next.js 15 (App Router, SSR, `output: "standalone"`) admin app of the marketplace, on its own host (ADR-0006, `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md`). It renders the operator screens and holds the admin session cookie. It owns no business capability, no database and no topics; the gateway enforces every scope on the data.

Status: compose service `team-frontend-admin`, port `:3002`. Stack: React 19, Tailwind 3, Connect-ES (`@connectrpc/connect-node`), `@fe-platform/{tokens,ui,api-client,session}` from vendored tarballs, Biome, Vitest.

## 1. Contract

**Serves** (HTTP only):

| Surface | What | Authorization |
|---|---|---|
| `/` | Redirects to `/admin/cockpit` | none |
| `/login` | Admin sign-in (no registration link, no sign-up route) | public |
| `/admin/cockpit`, `/admin/listings`, `/admin/cases`, `/admin/users`, `/admin/audit` | Ops cockpit, listing moderation, reports and disputes, user administration, audit log | `src/app/admin/layout.tsx`: `/login?returnUrl=` without a session, "Không có quyền" unless scope `admin`; each page and action re-checks, the gateway re-enforces |
| `/forbidden` | The "Không có quyền" page with a logout button | none |
| `GET /api/admin/metrics` | Cockpit refresh, proxies `fetchCockpit` | 401 without a session; the gateway enforces scope `admin` |

Isolation: `public/robots.txt` disallows everything; `next.config.mjs` sends `X-Robots-Tag: noindex, nofollow`, a CSP with `frame-ancestors 'none'` and `Referrer-Policy: strict-origin-when-cross-origin` on every route; every page also declares `referrer: no-referrer` (`src/app/referrerMetadata.test.ts`). The buyer app has no `/admin` route or link.

The screens keep the `/admin/*` paths on this host so `data-testid`s, roles, links between screens and the e2e page objects did not change.

**Consumes**: only `team-gateway` (`GATEWAY_URL`), through the clients in `src/lib/gateway/*` (`server-only`), built per request with the caller's token.

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

## 4. Known gaps

- Duplicated with `team-frontend` until shared pieces move to `fe-platform` (plan step D): `src/lib/gateway/` (client, auth interceptor, config, client context, refresh, revoke, token expiry, disputes, reports), `src/components/ui/` (facades over `@fe-platform/ui` plus local Badge, Descriptions, Empty, Modal, Table, Toast), `src/features/auth/` (login form, return URL, guard path), `src/features/dispute/` (resolve action, allowed actions), `src/lib/{action-result,useRouteRetry}.ts`, `src/components/LinkButton.tsx` and `src/lib/usePending.ts` (copied from the seller feature), `tailwind.config.ts`, `scripts/`, `proto/`.
- Platform voucher and campaign screens (plan E1c, E1d) are not built; their nav entries exist but stay hidden (`src/components/shell/nav.ts`).
- Ingress is not limited to a VPN or IP allow-list (platform-gitops, ADR-0006 open risk); no staging or prod overlay exists for this app yet.
- e2e: the admin scenarios now open `ADMIN_BASE_URL` (default `http://localhost:3002`) and seed the `admin_session` cookie, but they have not been run against this app; scenarios that expected GA4 pushes on admin pages no longer apply (this app ships no analytics).
