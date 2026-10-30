## Why

Three apps (buyer, seller, admin) will each sign users in and keep a session. Today the logic lives twice: in `team-frontend` (`src/middleware.ts`, `src/lib/gateway/session*.ts`, `token-expiry.ts`) and in the fe-platform template (`lib/session.ts`, `lib/guard.ts`, the refresh route). The backend rotates refresh tokens and revokes the session when one is reused, and browsers share cookies across ports on one host, so each app must use its own cookie names and refresh only its own cookies. One shared, tested package is the safe place for that rule (ADR-0006).

## What Changes

- New publishable package `@fe-platform/session` in `fe-platform/packages/session` (TypeScript source, peer deps `next` and `react`).
- Per-app configuration with presets: buyer (`session`, `agora_refresh`, `sameSite=lax`), seller (`seller_session`, `seller_refresh`, requires scope `listing.write`), admin (`admin_session`, `admin_refresh`, `sameSite=strict`, shorter lifetimes, requires scope `admin`); `__Host-` prefix over HTTPS for seller and admin; never a `Domain` attribute.
- Token helpers (usable, expired, malformed; principal and scopes decoded from the JWT for display and UX checks only).
- `createSessionMiddleware`: the refresh and strip-bad-token logic now in `team-frontend/src/middleware.ts`, made configurable and independent of generated stubs (the refresh call is injected; a default uses `@fe-platform/api-client`).
- Server helpers for pages and actions: `requireSession`, `guarded`, `attempt`, scope check with a "no access" redirect, `setSession`, `clearSession` that touches only the app's own cookies.
- The 5 behaviours from the plan are unit-tested: cookie names do not collide, concurrent refresh in two apps does not revoke either session, a buyer is turned away from seller and admin, bad tokens are stripped as today, signing out of one app leaves the others.
- Added to `npm run vendor` so the apps can consume it; `team-frontend` gets the dependency and `transpilePackages` entry and a smoke test. The app's own middleware is not replaced yet.

## Capabilities

### New Capabilities
none (shared infrastructure)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/session/**`, `fe-platform/scripts/vendor.mjs` (+test), `fe-platform/package.json`, `.changeset/`, `docs/agents/project.md`, `docs/gaps.md` (SSO hand-off gap).
- `team-frontend/package.json`, lockfile, `next.config.mjs`, `vendor/fe-platform-session.tgz`, one smoke test.
- No behaviour change in any running app.

## Non-goals

Switching `team-frontend` middleware or the template to the package (follow-up changes), seller and admin apps, SSO hand-off (needs a backend RPC), sign-in form components, CSRF tokens.

## Non-functional requirements

- Tokens are never logged; cookies are httpOnly and `Domain` is never set.
- Scope checks are UX only; the gateway stays the authority.
- Behaviour of the extracted middleware matches the existing one for the cases it covers (usable token passes, malformed or expired is stripped, refresh success rewrites both cookies, `Unauthenticated`/`PermissionDenied` clears both, transient failure keeps the refresh cookie).
