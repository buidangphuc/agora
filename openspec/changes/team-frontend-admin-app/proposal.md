## Why

ADR-0006 splits the storefront into three apps and puts admin on its own host. Plan P10 steps E1a, E1b and E1e build that admin app, move the five existing admin screens into it and remove every `/admin` route and link from the buyer app, so admin code, permissions and session cookies leave the public bundle.

## What Changes

- New `team-frontend-admin/` Next 15 app (port 3002) with the buyer app's toolchain: `check`, `build`, `dev`, `proto`, `test`, Biome, tsconfig, Vitest, Tailwind, Dockerfile that copies `vendor/` before `npm ci`, CI workflow `team-frontend-admin-ci.yml`.
- Session through `@fe-platform/session` with the `admin` preset: `admin_session`/`admin_refresh`, `sameSite=strict`, 30 min access and 8 h refresh, `__Host-` prefix on HTTPS; `src/middleware.ts` is `createSessionMiddleware`; admin login page without registration; the `/admin` layout sends guests to `/login` and shows "Không có quyền" without scope `admin`.
- Isolation: `public/robots.txt` disallows everything, `X-Robots-Tag: noindex, nofollow` on every route, CSP `frame-ancestors 'none'`, with tests.
- Moved from `team-frontend`: `admin/{cockpit,listings,cases,users,audit}` pages and tests, `features/admin`, `api/admin/metrics`, the dispute resolve action, `lib/gateway/{audit,cockpit,moderation,users}`; copied: the gateway helpers, ui facades, auth helpers, `LinkButton` and `usePending` they need. Admin shell with a nav to the five screens; voucher and campaign entries exist but stay hidden.
- Buyer app: no `src/app/(shop)/admin`, `src/features/admin`, `src/app/api/admin`; footer admin links, admin return-URL special case and the resolve action are gone; `src/app/admin-isolation.test.ts` fails on any `/admin` route or link.
- `fe-platform` vendor script writes and checks a list of app targets (buyer, seller, admin) and skips apps that do not exist yet.
- Infra: compose service `team-frontend-admin`, gateway `CORS_ORIGINS` and `TRUSTED_PROXIES` include the admin app, AGENTS.md port table and repo map, repo-doctor matches hyphenated repo names.
- platform-e2e: `ADMIN_BASE_URL` (default `http://localhost:3002`); admin page objects and steps use it and seed `admin_session`; new scenario "Admin isolation: /admin on the buyer app is not found".

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New `team-frontend-admin/`; `team-frontend/src` (admin removal), `team-frontend/FEATURES.yaml` (moved test paths, new entry), `fe-platform/scripts/vendor.mjs`, `fe-platform/package.json`, `docker-compose.services.yaml`, `team-gateway/.env.example`, `AGENTS.md`, `scripts/repo_doctor.py`, `.github/workflows/{team-frontend-admin-ci,fe-platform-ci}.yml`, `platform-e2e` config, pages, steps and one feature.
- Visible changes: `/admin/*` on port 3000 is a 404; admin is at `http://localhost:3002/admin/*`; buyer footer loses three admin links; signed-in non-admins see "Không có quyền" instead of "Cần tài khoản Quản trị".

## Non-goals

Voucher and campaign screens (E1c, E1d), the seller app (E2), cross-app sign-in hand-off (E4), gitops overlays and ingress limits, moving duplicated code to `fe-platform` (D), backend changes.

## Non-functional requirements

`npm run check`, `npm run build` and `docker build` green for `team-frontend-admin`; `npm run check` and `npm run build` green for `team-frontend`; `npm run check` green for `fe-platform`; data-testids, roles and accessible names of the moved screens unchanged.
