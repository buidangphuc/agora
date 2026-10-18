# login-return Specification

## Purpose
After login or register the user returns to the page they came from, through one returnUrl guard that rejects unsafe targets and is never sent to analytics.

## Requirements

### Requirement: returnUrl is validated by one guard

`safeReturnUrl(raw)` SHALL return `raw` unchanged only when it is a string of at most 2048 characters that starts with a single `/`, does not start with `//` or `/\`, contains no backslash, no ASCII control character and none of `%0d`, `%0a`, `%09`, `%5c` (any case), whose path (the part before the first `?` or `#`) does not begin with `/%2F%2F`, contains no scheme, and whose path is not `/login` or `/register` or beneath `/login/` or `/register/` (case-insensitive). For any other input, including `null`, `undefined` and an empty string, it SHALL return `/`.

#### Scenario: Same-origin path with query is kept
- **WHEN** `safeReturnUrl("/listing/abc?variant=v2")` is called
- **THEN** it returns `/listing/abc?variant=v2` unchanged
- **VERIFIED BY**: `team-frontend/src/features/auth/returnUrl.test.ts` `keeps a same-origin path with query`

#### Scenario: Absolute, protocol-relative and scheme URLs are rejected
- **WHEN** `safeReturnUrl` is called with `https://evil.example`, `//evil`, `/\evil`, `javascript:alert(1)` and `/\t/evil` (tab after the slash)
- **THEN** each call returns `/`
- **VERIFIED BY**: `returnUrl.test.ts` `rejects open-redirect shapes`

#### Scenario: Percent-encoded tricks are rejected
- **WHEN** `safeReturnUrl` is called with `/a%0d%0ab`, `/a%0Ab`, `/a%09b`, `/a%5Cb`, `/%5Cevil` and `/%2F%2Fevil`
- **THEN** each call returns `/`
- **AND** `/a%20b` and `/a%2Fb` are returned unchanged
- **VERIFIED BY**: `returnUrl.test.ts` `rejects percent-encoded tricks`

#### Scenario: Login and register targets are rejected to avoid a loop
- **WHEN** `safeReturnUrl` is called with `/login`, `/login?returnUrl=%2Fx`, `/Register`, `/register/`, `/LOGIN`, `/login#x` and `/login/x`
- **THEN** each call returns `/`
- **VERIFIED BY**: `returnUrl.test.ts` `rejects login and register targets`

#### Scenario: Paths merely prefixed by login or register are kept
- **WHEN** `safeReturnUrl` is called with `/loginfoo` and `/registered`
- **THEN** each is returned unchanged
- **VERIFIED BY**: `returnUrl.test.ts` `keeps look-alike prefixes`

#### Scenario: Missing, empty and oversize input fall back
- **WHEN** `safeReturnUrl` is called with `undefined`, `null`, `""`, and a 2049-character path starting with `/`
- **THEN** each call returns `/`, and a 2048-character path starting with `/` is returned unchanged
- **VERIFIED BY**: `returnUrl.test.ts` `falls back on missing and oversize input`

#### Scenario: loginPath encodes the return target
- **WHEN** `loginPath("/listing/abc?variant=v2")` is called
- **THEN** it returns `/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2`
- **AND** `loginPath("//evil")` and `loginPath(undefined)` return `/login` with no `returnUrl` parameter
- **VERIFIED BY**: `returnUrl.test.ts` `loginPath encodes and guards`

### Requirement: Login and register honour returnUrl

`loginAction` and `registerAction` SHALL, after a successful authentication, redirect to `safeReturnUrl(formData.get("returnUrl"))`. The `/login` and `/register` pages SHALL pass the validated `searchParams.returnUrl` to their forms, which SHALL submit it as a hidden `returnUrl` field and SHALL keep it on the link to the other page. On failed authentication or failed registration the action SHALL NOT redirect.

#### Scenario: Successful login returns to the requested page
- **WHEN** a user submits valid credentials on `/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2`
- **THEN** the browser lands on `/listing/abc?variant=v2`
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard redirect round trip` and `no auto replay after login`; unit `team-frontend/src/features/auth/actions.test.ts` `login redirects to returnUrl`

#### Scenario: Login without returnUrl goes home
- **WHEN** a user submits valid credentials with no `returnUrl` field
- **THEN** `loginAction` redirects to `/`
- **VERIFIED BY**: `actions.test.ts` `login without returnUrl redirects home`

#### Scenario: Tampered returnUrl is rejected server-side
- **WHEN** a user opens `/login?returnUrl=https://evil.example` (or tampers the hidden field) and submits valid credentials
- **THEN** the browser lands on `/` and never on the external host
- **VERIFIED BY**: e2e `login-return.spec.ts` `open redirect is rejected`; unit `actions.test.ts` `login ignores unsafe returnUrl`

#### Scenario: Failed login does not redirect
- **WHEN** the gateway answers Unauthenticated to `auth.login` with a `returnUrl` present
- **THEN** `loginAction` resolves `{ ok: false }` with the credentials error and does not call `redirect`
- **VERIFIED BY**: `actions.test.ts` `failed login keeps the form` (the gateway failure is not producible end to end without breaking credentials; covered by unit only)

#### Scenario: Register carries returnUrl through
- **WHEN** a new user submits the register form opened from `/register?returnUrl=%2Flisting%2Fabc`
- **THEN** `registerAction` redirects to `/listing/abc` after success
- **AND** the register page's link to login is `/login?returnUrl=%2Flisting%2Fabc`
- **VERIFIED BY**: `actions.test.ts` `register redirects to returnUrl`; `forms.test.tsx` `forms keep returnUrl on cross links`

#### Scenario: Register with an existing account does not redirect
- **WHEN** a user submits the register form with `returnUrl=/listing/abc` and the gateway answers `AlreadyExists`
- **THEN** `registerAction` resolves `{ ok: false }` with the already-exists error and does not call `redirect`
- **VERIFIED BY**: `actions.test.ts` `register already exists keeps the form`

### Requirement: Page guards pass their own path as returnUrl

Every server page or layout that redirects an unauthenticated visitor to `/login` SHALL call `redirectToLogin(<its own pathname plus query string>)`, producing `/login?returnUrl=<encoded path>`. This covers the seller, account, checkout, cart (including cart update and remove), favorites, notifications, chat and admin cockpit guards, including the cockpit's Unauthenticated-after-fetch branch.

Rebuild rule: path segments come from route params, each re-encoded with `encodeURIComponent`; the query string is built from `searchParams` in the object's key order, an array value emitting its key repeatedly in array order (`a=1&a=2`), keys and values encoded with `encodeURIComponent`, undefined values skipped. Layouts and pages without params (`seller/layout.tsx` and similar) SHALL read the `x-pathname` request header set by the `frontend-session-continuity` middleware. If the header is absent, a path cannot be rebuilt, or the resulting path plus query exceeds 2048 characters, the guard SHALL redirect to `/login` with no returnUrl.

#### Scenario: Guest opening a protected page is sent to login with its path
- **WHEN** an anonymous visitor requests `/account/orders/o-9`
- **THEN** the response is a redirect to `/login?returnUrl=%2Faccount%2Forders%2Fo-9`
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard redirect round trip` (asserts the login URL); unit `team-frontend/src/app/(shop)/account/orders/[id]/page.test.tsx` `redirects to login with returnUrl`

#### Scenario: Query string of the guarded page is preserved with repeated keys in order
- **WHEN** an anonymous visitor requests `/chat?thread=t1&tag=a&tag=b`
- **THEN** the response is a redirect to `/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db`
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard preserves query`; unit `team-frontend/src/app/(shop)/chat/page.test.tsx` `redirects to login with path and query`

#### Scenario: Param-less layout uses the x-pathname header
- **WHEN** an anonymous visitor requests `/seller/orders` and the middleware has set `x-pathname: /seller/orders`
- **THEN** `seller/layout.tsx` redirects to `/login?returnUrl=%2Fseller%2Forders`
- **AND** when the header is absent it redirects to `/login` with no returnUrl
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard redirect round trip` (seller variant); unit `team-frontend/src/app/(shop)/seller/layout.test.tsx` `uses x-pathname header`

#### Scenario: Over-length path falls back to plain login
- **WHEN** an anonymous visitor requests `/chat?thread=` followed by 2100 characters
- **THEN** the response is a redirect to `/login` with no `returnUrl` parameter
- **AND** a request whose rebuilt path plus query is exactly 2048 characters keeps its returnUrl
- **VERIFIED BY**: e2e `login-return.spec.ts` `over-length path falls back to login`; unit `returnUrl.test.ts` `redirectToLogin falls back past 2048`

#### Scenario: Cart page guard covers update and remove
- **WHEN** an anonymous visitor requests `/cart`
- **THEN** the response is a redirect to `/login?returnUrl=%2Fcart`
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard redirect round trip` (cart variant); unit `cart` page test `cart page redirects with returnUrl`

#### Scenario: Every guard uses the helper
- **WHEN** all occurrences under `team-frontend/src/app` (27 occurrences in 26 files at spec time) of `redirect("/login")` and `redirect("/login?` are searched
- **THEN** zero matches remain; all guards call `redirectToLogin`
- **VERIFIED BY**: `team-frontend/src/app/guards.test.ts` `no bare redirect to login` (static scan, cannot be produced end to end)

#### Scenario: Signed-in user without the scope is not redirected to login
- **WHEN** a signed-in buyer requests `/seller/new`
- **THEN** the page renders the "Cần tài khoản Người Bán" result (text present in `seller/layout.tsx`) and issues no redirect to `/login`
- **VERIFIED BY**: e2e `login-return.spec.ts` `buyer sees seller upgrade prompt`; unit `seller/layout.test.tsx` `buyer sees upgrade prompt not login`

#### Scenario: Returned user passes through the guard after login
- **WHEN** an anonymous visitor requests `/favorites`, logs in on the resulting login page
- **THEN** the browser lands on `/favorites`
- **VERIFIED BY**: e2e `login-return.spec.ts` `guard redirect round trip`

### Requirement: Guest add-to-cart and buy-now route to login with the product page

When `addToCartAction` receives `Code.Unauthenticated` from the gateway it SHALL resolve `{ ok: false, error, unauthenticated: true }`. On that result `PurchaseProvider` SHALL, for both "Thêm vào giỏ" and "Mua ngay", navigate to `loginPath("/listing/<listingId>?variant=<selectedVariantId>")`, omitting the `variant` parameter when the listing has no selected variant id. It SHALL NOT show the generic error toast, track `add_to_cart`, or replay the add after login. After a buy-now login the shopper lands on the PDP with no replay and not on checkout.

#### Scenario: Guest add-to-cart goes to login with the variant
- **WHEN** a guest on `/listing/abc?variant=v2` clicks "Thêm vào giỏ" and the gateway answers Unauthenticated
- **THEN** the browser navigates to `/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2`
- **VERIFIED BY**: e2e `login-return.spec.ts` `no auto replay after login`; unit `team-frontend/src/features/cart/PurchasePanel.test.tsx` `guest add to cart routes to login`

#### Scenario: Guest buy-now goes to login, then lands on the PDP
- **WHEN** a guest clicks "Mua ngay" and the gateway answers Unauthenticated, then logs in
- **THEN** the browser navigates to the login URL for the PDP, `/checkout` is never visited, and after login the browser lands on `/listing/<id>?variant=<v>` with zero `addToCart` RPCs issued
- **VERIFIED BY**: e2e `login-return.spec.ts` `guest buy now lands on PDP`; unit `PurchasePanel.test.tsx` `guest buy now routes to login`

#### Scenario: Listing without variants omits the variant parameter
- **WHEN** a guest clicks "Thêm vào giỏ" on `/listing/abc` whose selected variant id is empty
- **THEN** the browser navigates to `/login?returnUrl=%2Flisting%2Fabc`
- **VERIFIED BY**: `PurchasePanel.test.tsx` `guest add without variant` (needs a variant-less fixture listing; unit coverage)

#### Scenario: Other cart errors keep the toast
- **WHEN** the gateway answers `FailedPrecondition` (out of stock) to add-to-cart
- **THEN** the error toast is shown, no navigation happens and the result has no `unauthenticated` flag
- **VERIFIED BY**: `team-frontend/src/features/cart/actions.test.ts` `non-auth errors stay generic` (out-of-stock state is not reliably producible end to end)

#### Scenario: Add-to-cart is not replayed after login
- **WHEN** a guest returns to `/listing/abc?variant=v2` after logging in
- **THEN** zero `addToCart` RPCs are issued until the shopper clicks "Thêm vào giỏ" again, and the cart item count is unchanged
- **VERIFIED BY**: e2e `login-return.spec.ts` `no auto replay after login`

#### Scenario: Signed-in shopper is unaffected
- **WHEN** a signed-in shopper clicks "Thêm vào giỏ" and the gateway succeeds
- **THEN** the success toast is shown and no navigation to `/login` occurs
- **VERIFIED BY**: `PurchasePanel.test.tsx` `signed-in add still succeeds`

### Requirement: returnUrl is not sent to analytics

The analytics destination SHALL remove the `returnUrl` query parameter from `page_location` before pushing any event, so a `/login?returnUrl=...` page view reports `/login`.

#### Scenario: page_location has no returnUrl
- **WHEN** the login page is viewed at `/login?returnUrl=%2Faccount%2Forders%2Fo-9`
- **THEN** the pushed `page_location` equals the origin plus `/login` and contains no `returnUrl` and no `o-9`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/destinations/gtm.test.ts` `strips returnUrl from page_location` (dataLayer content is asserted at unit level)

#### Scenario: Other query parameters are not removed
- **WHEN** a page is viewed at `/search?q=ao&returnUrl=%2Fx&page=2`
- **THEN** `page_location` keeps `q=ao` and `page=2` and does not contain `returnUrl`; a URL without `returnUrl` is not changed
- **VERIFIED BY**: `gtm.test.ts` `keeps other params`
