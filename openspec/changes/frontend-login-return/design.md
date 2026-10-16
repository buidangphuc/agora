# Design

## Clarifications
### Round 1 — 2026-10-14
- Q7 [trigger] **middleware sets `x-pathname` request header; seller/layout.tsx uses it as returnUrl; path+query > 2048 chars or not rebuildable → `/login` without returnUrl** (default accepted)
- Q8 [data] (a) **strip `returnUrl` from GA4 page_location**; (b) buy-now login **lands on the PDP, no replay**; (c) **cart page guard covers cart update/remove** (default accepted)

## Decisions (round 2)
- `safeReturnUrl` splits the value at the first `?` or `#` to get the path. Path `/login` or `/register`, or beginning `/login/` or `/register/`, is rejected case-insensitively (`/LOGIN`, `/login#x`, `/login/x` rejected; `/loginfoo`, `/registered` kept).
- Percent-encoded tricks: reject if the value contains `%0d`, `%0a`, `%09` or `%5c` (any case) anywhere, or its path begins `/%2F%2F`.
- Guard rebuild: path = route segments each re-encoded with `encodeURIComponent`, joined by `/`. Query = searchParams keys in the object's iteration order; an array value emits the key repeatedly in array order (`?a=1&a=2`); each key and value goes through `encodeURIComponent`; undefined values are skipped. Param-less layouts use the `x-pathname` header (assumed to carry pathname plus query string as received). Any missing piece, or a result over 2048 chars, gives `redirectToLogin(undefined)` = `/login`.
- GA4: the analytics destination (`src/lib/analytics/destinations/gtm.ts`) removes the `returnUrl` query parameter from `page_location` before pushing.
