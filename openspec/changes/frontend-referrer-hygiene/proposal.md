## Why

Admin URLs carry personal and operational identifiers in the query (`/admin/users?query=<email>`, `/admin/audit?target_id=<id>&actor_id=<id>`). Today no `Referrer-Policy` header exists (`next.config.mjs` sets only CSP), and only `/admin/users` sets `referrer: no-referrer` metadata. So `/admin/audit`, `/admin/listings`, `/admin/cases` and `/admin/cockpit` leak their full URL through the `Referer` header to any cross-origin request, GA4 `page_location` carries the admin query (the GTM destination strips only `returnUrl`), and GA4's automatic `page_referrer` reads `document.referrer`, which after a login redirect is the full `/login?returnUrl=...`. A guest hitting `/admin/users?query=...` also gets the whole query copied into `returnUrl`.

## What changes

- `next.config.mjs` adds a global `Referrer-Policy: strict-origin-when-cross-origin` header next to the CSP header.
- `/login` and every `/admin/*` page (`cockpit`, `users`, `audit`, `listings`, `cases`) export `referrer: "no-referrer"` metadata.
- `src/lib/analytics/destinations/gtm.ts`: `page_location` drops ALL query parameters and the fragment on `/admin/*` (`returnUrl` stripping stays for every other path); a scrubbed `page_referrer` (scheme + host + path, via `scrubReferrer`) is pushed together with every `page_location`.
- `src/features/auth/guardPath.ts`: guest redirects for `/admin/*` keep only the path in `returnUrl` (drops searchParams, and the query of the `x-pathname` header value).
- Existing FEATURES id `auth.login-return.page-location-has-no-returnurl` keeps passing unchanged.

initiative: privacy-audit-hardening

Depends on `frontend-cookie-secure` (serialised on shared frontend hot files). No proto or RPC change.

## NFR

- No added network call; header and metadata are static. Page TTFB and `page_location` push latency unchanged (computation is a `new URL` parse, under 1 ms).
- No availability impact; a failure to parse a URL pushes an empty `page_referrer` and never throws.
- Limits: `returnUrl` length cap (2048) unchanged; the admin path-only form is always shorter than the cap.

## Data classification

| field | classification | purpose | consent basis | retention |
|---|---|---|---|---|
| Admin URL query (emails, user ids, target/actor ids) | personal / sensitive operational | none after this change (must not leave the origin) | n/a, no longer collected by GA4 or referrers | not stored |
| `page_location` (admin paths, path only) | non-personal | analytics page view | analytics consent (existing `analytics_storage` gate) | GA4 property setting, unchanged |
| `page_referrer` (scheme+host+path) | non-personal after scrub | analytics attribution | analytics consent (same gate) | GA4 property setting, unchanged |
| `returnUrl` for admin guard | non-personal (path only) | post-login navigation | functional, no consent needed | URL lifetime only |

## Threat model

- Assets: admin search terms and entity ids in URLs; user emails typed into `/admin/users?query=`.
- Abuse cases: (1) admin page loads a cross-origin asset or follows a link and the full URL reaches that third party via `Referer`; (2) GA4 stores the admin query in `page_location` or the login URL in `page_referrer`; (3) a guest link `/admin/users?query=x` bounces through `/login?returnUrl=` and spreads the query into logs, history and referrers.
- Authz rule: unchanged. Guests still go to login, non-admins still see the 403 view. This slice only removes data from outbound channels; it grants no access.
- Residual: same-origin navigation and server access logs still show the query (out of scope); third-party scripts reading `location.href` are bounded by the CSP.
