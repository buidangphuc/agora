## MODIFIED Requirements

### Requirement: Page guards pass their own path as returnUrl

Every server page or layout that redirects an unauthenticated visitor to `/login` SHALL call `redirectToLogin(<its own pathname plus query string>)`, producing `/login?returnUrl=<encoded path>`. This covers the seller, account, checkout, cart (including cart update and remove), favorites, notifications, chat and admin cockpit guards, including the cockpit's Unauthenticated-after-fetch branch.

Rebuild rule: path segments come from route params, each re-encoded with `encodeURIComponent`; the query string is built from `searchParams` in the object's key order, an array value emitting its key repeatedly in array order (`a=1&a=2`), keys and values encoded with `encodeURIComponent`, undefined values skipped. Layouts and pages without params (`seller/layout.tsx` and similar) SHALL read the `x-pathname` request header set by the `frontend-session-continuity` middleware. If the header is absent, a path cannot be rebuilt, or the resulting path plus query exceeds 2048 characters, the guard SHALL redirect to `/login` with no returnUrl.

#### Scenario: Guest opening a protected page is sent to login with its path
- **WHEN** an anonymous visitor requests `/account/orders/o-9`
- **THEN** the response is a redirect to `/login?returnUrl=%2Faccount%2Forders%2Fo-9`
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Guest opening a protected page is sent to login with its path`; unit `team-frontend/src/app/(shop)/account/orders/[id]/page.test.tsx` `redirects to login with returnUrl`

#### Scenario: Query string of the guarded page is preserved with repeated keys in order
- **WHEN** an anonymous visitor requests `/chat?thread=t1&tag=a&tag=b`
- **THEN** the response is a redirect to `/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db`
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Query string of the guarded page is preserved with repeated keys in order`; unit `team-frontend/src/app/(shop)/chat/page.test.tsx` `redirects to login with path and query`

#### Scenario: Param-less layout uses the x-pathname header
- **WHEN** an anonymous visitor requests `/seller/orders` and the middleware has set `x-pathname: /seller/orders`
- **THEN** `seller/layout.tsx` redirects to `/login?returnUrl=%2Fseller%2Forders`
- **AND** when the header is absent it redirects to `/login` with no returnUrl
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Param-less layout uses the x-pathname header`; unit `team-frontend-seller/src/app/seller/layout.test.tsx` `uses x-pathname header`

#### Scenario: Over-length path falls back to plain login
- **WHEN** an anonymous visitor requests `/chat?thread=` followed by 2100 characters
- **THEN** the response is a redirect to `/login` with no `returnUrl` parameter
- **AND** a request whose rebuilt path plus query is exactly 2048 characters keeps its returnUrl
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Over-length path falls back to plain login`; unit `returnUrl.test.ts` `redirectToLogin falls back past 2048`

#### Scenario: Cart page guard covers update and remove
- **WHEN** an anonymous visitor requests `/cart`
- **THEN** the response is a redirect to `/login?returnUrl=%2Fcart`
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Cart page guard covers update and remove`; unit `cart` page test `cart page redirects with returnUrl`

#### Scenario: Every guard uses the helper
- **WHEN** all occurrences under `team-frontend/src/app` (27 occurrences in 26 files at spec time) of `redirect("/login")` and `redirect("/login?` are searched
- **THEN** zero matches remain; all guards call `redirectToLogin`
- **VERIFIED BY**: `team-frontend/src/app/guards.test.ts` `no bare redirect to login` (static scan, cannot be produced end to end)

#### Scenario: Signed-in user without the scope is not redirected to login
- **WHEN** a signed-in buyer requests `/seller/new`
- **THEN** the page renders the seller onboarding screen with the button "Đồng ý và trở thành Người Bán" (component rendered by `seller/layout.tsx`) and issues no redirect to `/login`
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Signed-in user without the scope is not redirected to login`; unit `seller/layout.test.tsx` `buyer sees onboarding not login`

#### Scenario: Returned user passes through the guard after login
- **WHEN** an anonymous visitor requests `/favorites`, logs in on the resulting login page
- **THEN** the browser lands on `/favorites`
- **VERIFIED BY**: e2e `platform-e2e/tests/e2e/features/auth/login_return.feature` `Returned user passes through the guard after login`
