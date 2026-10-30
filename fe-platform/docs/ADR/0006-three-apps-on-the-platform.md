# ADR-0006: Three frontend apps on the shared platform

Status: accepted, 2026-10-30 (chosen by the user; supersedes the P0 decision "no mapping into team-frontend"). Implemented 2026-10-30 on the stacked P10 branches (`feat/team-frontend-f` and its parents), not yet merged to `main`; deployment is not wired (see Open risks).

## Decision

- The storefront is split into three Next apps with their own Docker image, host and deploy: the buyer app (`team-frontend`, keeps `(shop)`), `team-frontend-seller` (`seller/*`, `sell`) and `team-frontend-admin` (`admin/*`). Ports locally: 3000, 3001, 3002.
- `fe-platform` holds only what is shared and important: `tokens`, the core `ui` components (more only when two apps use them), `api-client`, and a `session` package (cookie handling, refresh, guards, scope checks). Business features (`features/*`), single-app components and pages stay in the apps. Apps never import each other's code.
- Package delivery is vendored tarballs. `npm run vendor` in `fe-platform` packs `tokens`, `ui`, `api-client` and `session` into each app's `vendor/` under stable names and the apps depend on `file:./vendor/*.tgz`. The tarballs are committed. `npm run vendor:check` fails the platform gate when a tarball differs from a fresh pack of the sources.
- Sessions are per app. Each app keeps its own httpOnly cookies, never sets `Domain`, and uses distinct names: the buyer app keeps `session` and `agora_refresh`; seller uses `seller_session` and `seller_refresh`; admin uses `admin_session` and `admin_refresh`. Over HTTPS the `__Host-` prefix applies. Admin cookies are `sameSite=strict` with a short lifetime. Each app checks the scopes it needs after sign-in; the gateway stays the real authority.
- The admin app lives on its own host. The buyer and seller apps carry no `/admin` route and no link to it (404, no redirect). Admin serves `robots.txt` disallowing everything and `X-Robots-Tag: noindex, nofollow`, has no self sign-up, and signs in only on its own host. Limiting its ingress to a VPN or an IP allow-list belongs to platform-gitops and is deferred; until then it is an open risk.
- Merge order: `fe-platform` first, then the buyer app, then seller and admin.

## Why

- Each audience has a different risk profile and release rhythm; one app forced admin and seller code and permissions into the public bundle and its attack surface.
- `team-frontend`'s image builds from its own directory only, so a workspace link or a registry on 127.0.0.1 would break `npm ci`; vendored tarballs need nothing beyond npm and are verifiable byte for byte.
- The backend rotates refresh tokens and treats reuse as theft, revoking the session. Three apps sharing one refresh cookie would race and sign users out; browsers also share cookies across ports on one host, so distinct names are needed even in local development. Separate sessions also keep admin tokens out of the buyer app's reach.
- Sharing only tokens, core components, the client and sessions keeps the platform small enough to govern (ADR-0001, ADR-0004, ADR-0005).

## Consequences

- A person who both buys and sells signs in once per app. A one-time-code hand-off through identity would remove that; it needs a backend RPC and is recorded as a gap (`docs/gaps.md`) for later.
- Committed tarballs add binary files to git. The long-term fix is a shared authenticated registry (ADR-0004); then only the dependency lines change.
- Updating a shared package means `npm run vendor` and committing the tarballs and the lockfiles of the apps that use it.
- The platform must merge before the apps that depend on its tarballs.
- Only about 8 of the 37 buyer-app components have a platform equivalent; the rest are restyled with the new tokens inside the apps and promoted only when two apps need them.
- The admin app keeps the `/admin/*` paths on its own host (its root redirects to `/admin/cockpit`), so test ids, links between screens and e2e page objects carry over; the e2e suite reaches it through `ADMIN_BASE_URL`. Code the admin and buyer apps both need (gateway helpers, ui facades, auth forms) is duplicated until step D promotes it, and listed in the admin README.

## What was built

- `fe-platform`: the `session` package (cookie presets `buyer`, `seller`, `admin`, refresh-once middleware, guards, scope checks) next to `tokens`, `ui` and `api-client`; `npm run vendor` writes all four tarballs into the three apps listed in `vendor.targets`, and `vendor:check` guards them. Core components promoted to `@fe-platform/ui` when two apps needed them (step D).
- `team-frontend-seller` (`:3001`): the seller centre (`/seller/*`, `/sell`), its own login with `?next=`, `/forbidden` without `listing.write`, `seller_session`/`seller_refresh`, logout that clears only those cookies.
- `team-frontend-admin` (`:3002`): moderation, cases and disputes, users, audit, cockpit, plus new screens for platform vouchers, flash-sale campaigns and seller plans (read only); its own login without sign-up, `admin_session`/`admin_refresh` (`sameSite=strict`, short TTL), `robots.txt` disallow, `X-Robots-Tag: noindex, nofollow`, `frame-ancestors 'none'` and `referrer: no-referrer` on every page.
- Buyer app: `/seller*`, `/sell` and `/admin` are a 404 with tests that fail on a reintroduced route; cross-app links go through `src/lib/apps.ts` (`NEXT_PUBLIC_SELLER_APP_URL` in the buyer app, `NEXT_PUBLIC_BUYER_APP_URL` in the seller app; the admin app links to neither).
- Layout passes on `@fe-platform/ui` for the buyer, seller and admin pages, keeping `data-testid`s, roles and accessible names.
- Infrastructure: three compose services; the gateway `CORS_ORIGINS` and `TRUSTED_PROXIES` in compose list all three apps; one CI workflow per app (`check`, `build`, `docker build` from the app directory); gitops baselines and staging/prod overlays per app with `COOKIE_SECURE: "true"`.
- Tests and specs: platform-e2e routes seller pages to `SELLER_BASE_URL` and admin pages to `ADMIN_BASE_URL`; the feature manifest is split into `team-frontend`, `team-frontend-seller` and `team-frontend-admin` `FEATURES.yaml`, and `openspec/specs` point at the tests in their new app.

## Open risks

- Admin ingress is not limited to a VPN or an IP allow-list; the admin host is reachable by anyone who knows it, protected only by sign-in, scope `admin` and the gateway.
- The seller and admin apps are not wired into ArgoCD: no `argocd/apps/` Application, no entry in `appset-services.yaml`, no image tag in `envs/*/values.yaml` (`platform-gitops/README.md`). The deployed gateway `TRUSTED_PROXIES` still names only `team-frontend`, so once they deploy, the gateway records their pod IP instead of the browser IP on login and refresh.
- No single sign-on (gap G8 in `docs/gaps.md`): users sign in once per app until identity offers a one-time hand-off RPC; admin stays excluded from any hand-off.
- Gateway helpers, auth forms, UI facades and the vendored `proto/` are duplicated in the three apps until promoted; a fix in one must be repeated in the others.
- Seller and admin `OTEL_SERVICE_NAME` default to `team-frontend`, so their gateway calls look like the buyer app until each environment sets it.
- The admin and seller e2e scenarios target the new hosts but have not all run end to end against them; the containerised e2e runner does not set `ADMIN_BASE_URL`.

