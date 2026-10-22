## Why

A guest who clicks "Thêm vào giỏ" or opens a protected page is sent to `/login` and, after login, lands on `/`, losing their place. Today only `chat/page.tsx` and three listing links pass `returnUrl`, unencoded, and `loginAction`/`registerAction` ignore it (`team-frontend/src/features/auth/actions.ts` always `redirect("/")`). 27 occurrences of a bare `redirect("/login")` in 26 files under `src/app` (at spec time) do this. `addToCartAction` swallows the gateway's Unauthenticated into a generic error toast, so a guest never reaches login at all. Accepting a `returnUrl` without a guard is an open redirect, so the guard ships with the feature.

initiative: q1-session-continuity

depends_on: frontend-session-continuity (its middleware sets the `x-pathname` request header; see Uses).

## What Changes

- New `safeReturnUrl(raw)` in `team-frontend/src/features/auth/returnUrl.ts`: the single open-redirect guard. Also `loginPath(returnTo?)` that builds `/login?returnUrl=<encoded>` and `redirectToLogin(returnTo)` for server guards.
- `/login` and `/register` pages read `searchParams.returnUrl`, validate it, and pass it to the forms. Forms submit it as a hidden `returnUrl` field; the cross-links between login and register keep it. `loginAction` and `registerAction` redirect to `safeReturnUrl(formData.returnUrl)`.
- Every page/layout guard that does `redirect("/login")` (seller, account, checkout, favorites, notifications, admin cockpit, chat) calls `redirectToLogin(<own path + query>)`. Existing unencoded `returnUrl` links (chat page, `ChatWithSellerButton`, `QASection`, `QuestionList`) move to the same helper.
- Guards without route params (`seller/layout.tsx` and the other param-less layouts) read the `x-pathname` request header, set by the middleware of slice `frontend-session-continuity`, via `headers()` to build their returnUrl. If the header is absent, the rebuilt path+query exceeds 2048 characters, or a path cannot be rebuilt, the guard redirects to plain `/login` with no returnUrl.
- The cart page guard (`/cart`, covering cart update and remove) uses the same helper.
- GA4: `returnUrl` is stripped from the `page_location` sent to analytics (the `/login` page view reports `/login`).
- `addToCartAction` returns `{ ok: false, error, unauthenticated: true }` when the gateway answers `Code.Unauthenticated`. `PurchaseProvider.add()` (add-to-cart and buy-now) then navigates to `loginPath("/listing/<id>?variant=<selected variant>")`.
- No auto-replay: after login the shopper lands on the PDP and clicks again. No cart cookie, no cart merge.
- No proto, gateway or identity change. Independent of the refresh-token slices.

Shared file: `team-frontend/src/features/auth/actions.ts` is also edited by `frontend-session-continuity` (cookie writes, logout). This slice changes only the `redirect("/")` targets and reads the `returnUrl` form field; the build serialises the two slices.

## NFR

- `safeReturnUrl` is a pure synchronous function, no I/O, O(length); input over 2048 chars falls back to `/`. Guards fall back to `/login` without returnUrl past the same limit.
- No added network call on the login path; the redirect target is decided in the same server action.
- A guard redirect adds only a query string to an existing 307; no latency budget change.

## Data classification

- `returnUrl` (a same-origin path plus query, e.g. `/listing/abc?variant=v2`). Purpose: navigation after login. Personal data: none by design, but paths such as `/account/orders/<id>` carry an order id. Basis: legitimate interest / contract (service operation). Retention: not persisted by the app (no cookie, no database, no local storage) and not sent to analytics (stripped from GA4 `page_location`); it is visible in request URLs and therefore in gateway/CDN access logs under the existing access-log retention.
- No new cookies, no new fields to analytics.

## Uses

- `x-pathname` request header from the `frontend-session-continuity` middleware (Node runtime), read by guards without params.

## Threat model

- Assets: shopper session (the post-login redirect must not send a freshly authenticated browser to an attacker page); order ids embedded in return paths.
- Abuse cases:
  1. Open redirect via `returnUrl=https://evil.example`, `//evil`, `/\evil`, `javascript:`, or `/%0d%0a`, `%09`, `%5C` and `/%2F%2F` percent-encoded tricks and raw tab/newline tricks (`/\t/evil` is parsed by browsers as `//evil`).
  2. Redirect loop via `returnUrl=/login` or `/register`.
  3. Tampered hidden `returnUrl` on the form POST; the server action revalidates and never trusts the client or the page-side value.
  4. Return path to a page the user may not access: the target page's own guard still applies; `returnUrl` grants nothing.
- Authz rule: `returnUrl` is navigation only. Authorization stays in the target page guard and the gateway. Only same-origin relative paths are ever redirected to.
