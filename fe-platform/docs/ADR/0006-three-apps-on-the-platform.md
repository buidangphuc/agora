# ADR-0006: Three frontend apps on the shared platform

Status: accepted, 2026-10-30 (chosen by the user; supersedes the P0 decision "no mapping into team-frontend").

## Decision

- The storefront is split into three Next apps with their own Docker image, host and deploy: the buyer app (`team-frontend`, keeps `(shop)`), `team-frontend-seller` (`seller/*`, `sell`) and `team-frontend-admin` (`admin/*`). Ports locally: 3000, 3001, 3002.
- `fe-platform` holds only what is shared and important: `tokens`, the core `ui` components (more only when two apps use them), `api-client`, and a `session` package (cookie handling, refresh, guards, scope checks). Business features (`features/*`), single-app components and pages stay in the apps. Apps never import each other's code.
- Package delivery is vendored tarballs. `npm run vendor` in `fe-platform` packs `tokens`, `ui` and `api-client` into each app's `vendor/` under stable names and the apps depend on `file:./vendor/*.tgz`. The tarballs are committed. `npm run vendor:check` fails the platform gate when a tarball differs from a fresh pack of the sources.
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
