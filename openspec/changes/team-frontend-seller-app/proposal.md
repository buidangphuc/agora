## Why

ADR-0006 splits the frontend into three Next apps (buyer, seller, admin) with their own images, hosts and sessions. Plan step E2 moves the seller centre out of `team-frontend` into its own app on port 3001, so the buyer app stops carrying seller routes and the seller app gets a session that cannot collide with the buyer one.

## What Changes

- New top-level app `team-frontend-seller/` (Next 15, port 3001) with the same toolchain as `team-frontend`: `package.json` scripts, Biome, tsconfig, Vitest, Tailwind/PostCSS, vendored proto with `buf generate` into a gitignored `src/generated`, Dockerfile that copies `vendor/` before `npm ci`, `.dockerignore`, `.env.example`, README.
- Moved from `team-frontend`: `src/app/(shop)/seller/**` (now `src/app/seller/**`), `src/app/(shop)/sell` (now `src/app/sell`), `src/features/seller/**`, and the seller-only listing and order pieces (`ListingForm`, `MagicListingAssist`, `DeleteListingModal`, `ModerationBanner`, the listing save/delete/upload/magic actions, `updateOrderStatusAction`), with their Vitest tests. Gateway clients, UI facades, auth form/actions and a few helpers are copied (listed in the README known gaps).
- Session: `@fe-platform/session` preset `seller` (`seller_session`/`seller_refresh`, `__Host-` prefix on HTTPS, no `Domain`), middleware from `createSessionMiddleware`, seller login page, and a `/forbidden` "Không có quyền" page for a signed-in user without `listing.write` that links to `/sell` and back to the buyer app.
- Seller links to buyer pages (public listing, public shop, chat, home) go through `buyerAppUrl()` (`NEXT_PUBLIC_BUYER_APP_URL`, default `http://localhost:3000`).
- `team-frontend`: seller routes and features deleted, so `/seller*` and `/sell` are the standard 404; header links use `sellerAppUrl()` (`NEXT_PUBLIC_SELLER_APP_URL`, default `http://localhost:3001`); `src/app/sellerRoutes.test.ts` fails on an internal `/seller` or `/sell` link. `features/seller/LinkButton.tsx` and `usePending.ts` stay because admin pages and `ReportListingButton` still import them.
- `fe-platform`: `vendor.targets` lists the buyer, seller and admin apps; `npm run vendor` and `vendor:check` skip a target whose app directory does not exist.
- Infra: compose service `team-frontend-seller` (3001), gateway `CORS_ORIGINS` and `team-gateway/.env.example` add `http://localhost:3001` and `http://team-frontend-seller:3001`, AGENTS.md repo row and ports, CI workflow `team-frontend-seller-ci.yml`, `fe-platform-ci.yml` path filter, `repo_doctor.py` parses hyphenated app names, `asdlc.config.yaml` routes the seller session files to the auth reviewer.
- `platform-e2e`: `SELLER_BASE_URL` setting (default `http://localhost:3001`), seller page objects set `app = "seller"`, `tests/e2e/support/apps.py` routes `/seller*` and `/sell` to the seller app, API logins inject both `session` and `seller_session`.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New `team-frontend-seller/`; `team-frontend/src` (deletions, header links, two tests); `fe-platform/scripts/vendor.mjs`, `fe-platform/package.json`; `docker-compose.services.yaml`, `team-gateway/.env.example`, `AGENTS.md`, `.github/workflows/*`, `scripts/repo_doctor.py`, `asdlc.config.yaml`; `platform-e2e` settings, base page, seller pages and seller step files.
- Users: the seller centre moves from `http://localhost:3000/seller` to `http://localhost:3001/seller`; a buyer signed in on 3000 signs in again on 3001.

## Non-goals

Admin app and removal of `/admin` (plan E1), SSO between apps (gap G8), gitops overlays and `COOKIE_SECURE` manifests for the seller app (E4), seller page relayout (F), moving the seller features out of `team-frontend/FEATURES.yaml`.

## Non-functional requirements

`npm run check` and `npm run build` green in both apps; `docker build` of the seller app from its own directory; `npm run check` green in `fe-platform`; e2e Python modules import; no new third-party dependency.
