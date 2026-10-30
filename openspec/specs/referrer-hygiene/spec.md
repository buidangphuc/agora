# referrer-hygiene Specification

## Purpose
Login and admin URLs (returnUrl, admin queries) do not leak through the Referrer header, GA4 page_location/page_referrer or the guest returnUrl.

## Requirements

### Requirement: Global referrer policy header

The frontend SHALL send `Referrer-Policy: strict-origin-when-cross-origin` on every response matched by `/:path*`, in addition to the existing `Content-Security-Policy` header.

#### Scenario: Referrer hygiene: every response carries the policy header
- **WHEN** any page (`/`, `/login`, `/admin/audit`) is requested
- **THEN** the response contains the header `Referrer-Policy` with the value `strict-origin-when-cross-origin`
- **AND** the `Content-Security-Policy` header is still present
- **VERIFIED BY**: `team-frontend/next.config.test.ts` `headers() sets Referrer-Policy and keeps CSP`

#### Scenario: Referrer hygiene: a weaker Referrer-Policy value is not configured
- **WHEN** the `headers()` result is inspected for all sources
- **THEN** exactly one `Referrer-Policy` entry exists, and the configuration does not contain a weaker value such as `unsafe-url`, `no-referrer-when-downgrade` or `origin-when-cross-origin`
- **VERIFIED BY**: `next.config.test.ts` `weaker Referrer-Policy is not configured`

### Requirement: Login and admin pages send no referrer

`/login` and every page under `/admin/` (`cockpit`, `users`, `audit`, `listings`, `cases`) SHALL export `metadata.referrer = "no-referrer"`, so the rendered document contains `<meta name="referrer" content="no-referrer">`.

#### Scenario: Referrer hygiene: admin audit page declares no-referrer
- **WHEN** `/admin/audit?target_id=u-1` is rendered for an admin
- **THEN** the document head contains `<meta name="referrer" content="no-referrer">`
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/audit/page.test.tsx` `metadata referrer is no-referrer`

#### Scenario: Referrer hygiene: every admin page and login declare no-referrer
- **WHEN** the metadata export of `/login` and of `admin/cockpit`, `admin/users`, `admin/audit`, `admin/listings`, `admin/cases` is read
- **THEN** each has `referrer` equal to `no-referrer`
- **VERIFIED BY**: `team-frontend-admin/src/app/referrerMetadata.test.ts` `login and all admin pages set no-referrer`

#### Scenario: Referrer hygiene: a new admin page without the metadata fails the build
- **WHEN** a `page.tsx` exists under `team-frontend-admin/src/app/admin/` whose metadata lacks `referrer: "no-referrer"`
- **THEN** the test `every admin page sets no-referrer` fails and names the file
- **VERIFIED BY**: `team-frontend-admin/src/app/referrerMetadata.test.ts` `every admin page sets no-referrer` (static scan, not producible end to end)

#### Scenario: Referrer hygiene: non-admin 403 view on an admin URL also sends no referrer
- **WHEN** a signed-in buyer opens `/admin/users?query=x`
- **THEN** the 403 view is rendered and the document head contains `<meta name="referrer" content="no-referrer">`
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/users/page.test.tsx` `403 view keeps no-referrer`

#### Scenario: Referrer hygiene: no admin query reaches a cross-origin request header
- **WHEN** an admin opens `/admin/audit?target_id=u-1` in a browser and the page issues a cross-origin request
- **THEN** that request carries no `Referer` header
- **VERIFIED BY**: e2e `referrer hygiene admin page sends no referer` (`team-frontend/e2e/referrer-hygiene.spec.ts`)

### Requirement: GTM page_location hides admin queries

The GTM destination SHALL push `page_location` with all query parameters and the fragment removed when the path is `/admin` or begins with `/admin/`; for every other path it SHALL keep removing only `returnUrl` and keep other parameters.

#### Scenario: Referrer hygiene: admin page_location is path only
- **WHEN** consent is granted and the browser is at `https://shop.example/admin/audit?target_id=u-1&actor_id=u-2#x`
- **THEN** the pushed `page_location` equals `https://shop.example/admin/audit`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/destinations/gtm.test.ts` `admin page_location drops all query and fragment`

#### Scenario: Referrer hygiene: look-alike path is not treated as admin
- **WHEN** the browser is at `/administrator?q=1` and at `/adminx?q=1`
- **THEN** the pushed `page_location` keeps `?q=1` in both cases
- **VERIFIED BY**: `gtm.test.ts` `admin match is segment exact`

#### Scenario: Referrer hygiene: the admin root is stripped too
- **WHEN** the browser is at `/admin?x=1`, at `/ADMIN/users?x=1` and at `/admin/users/?x=1`
- **THEN** the pushed `page_location` has no query in each case
- **VERIFIED BY**: `gtm.test.ts` `admin root and case variants stripped`

#### Scenario: Referrer hygiene: non-admin pages keep other params
- **WHEN** the browser is at `/listing/abc?variant=v2&returnUrl=%2Fx`
- **THEN** the pushed `page_location` is `/listing/abc?variant=v2`
- **VERIFIED BY**: `gtm.test.ts` `keeps other params` (existing, must stay green)

#### Scenario: Referrer hygiene: login page_location still has no returnUrl
- **WHEN** the login page is viewed at `/login?returnUrl=%2Faccount%2Forders%2Fo-9`
- **THEN** the pushed `page_location` equals origin plus `/login` with no `returnUrl` and no `o-9`
- **VERIFIED BY**: `gtm.test.ts` `strips returnUrl from page_location` (existing, FEATURES id `auth.login-return.page-location-has-no-returnurl`)

#### Scenario: Referrer hygiene: non-admin signed-in user on an admin URL gets the stripped location too
- **WHEN** a signed-in buyer with consent is at `/admin/users?query=x` (403 view shown)
- **THEN** the pushed `page_location` equals the origin plus `/admin/users`
- **VERIFIED BY**: `gtm.test.ts` `admin page_location drops all query and fragment` (path rule is independent of the principal)

#### Scenario: Referrer hygiene: no push without analytics consent
- **WHEN** the latest consent update does not grant `analytics_storage` and a navigation occurs
- **THEN** no `page_location` and no `page_referrer` entry is pushed
- **VERIFIED BY**: `gtm.test.ts` `no push without consent`

### Requirement: GTM pushes a scrubbed page_referrer

Every `page_location` push by the GTM destination SHALL also include `page_referrer` equal to `scrubReferrer(document.referrer)` (scheme, host and path only), or the empty string when the referrer is empty or unparseable.

#### Scenario: Referrer hygiene: login URL referrer loses returnUrl
- **WHEN** a user lands on `/` after login and `document.referrer` is `https://shop.example/login?returnUrl=%2Fadmin%2Fusers%3Fquery%3Da%40b.vn`
- **THEN** the pushed `page_referrer` equals `https://shop.example/login`
- **VERIFIED BY**: `gtm.test.ts` `page_referrer drops login returnUrl`

#### Scenario: Referrer hygiene: admin referrer loses its query
- **WHEN** `document.referrer` is `https://shop.example/admin/audit?target_id=u-1`
- **THEN** the pushed `page_referrer` equals `https://shop.example/admin/audit`
- **VERIFIED BY**: `gtm.test.ts` `page_referrer drops admin query`

#### Scenario: Referrer hygiene: empty or invalid referrer becomes empty string
- **WHEN** `document.referrer` is `""` and, in another run, `not a url`
- **THEN** the pushed `page_referrer` equals `""` in both runs and no exception is thrown
- **VERIFIED BY**: `gtm.test.ts` `page_referrer empty on invalid input`

#### Scenario: Referrer hygiene: dataLayer after an admin visit has no admin query
- **WHEN** a consenting admin opens `/admin/audit?target_id=u-1` in a browser and navigates to `/admin/users?query=a%40b.vn`
- **THEN** no `dataLayer` entry contains `target_id`, `query=` or `a@b.vn` in `page_location` or `page_referrer`
- **VERIFIED BY**: e2e `referrer hygiene admin datalayer is query free` (`team-frontend/e2e/referrer-hygiene.spec.ts`)

### Requirement: Guest returnUrl for admin pages keeps only the path

When a guest is redirected to login from a path equal to `/admin` or beneath `/admin/`, `buildReturnPath` and `redirectGuestToLoginFromHeader` SHALL produce a `returnUrl` containing only the path, with search parameters and fragment omitted. Other paths SHALL keep their query as before.

#### Scenario: Referrer hygiene: guest on admin users search gets path-only returnUrl
- **WHEN** a guest opens `/admin/users?query=a%40b.vn&state=banned`
- **THEN** the browser is redirected to `/login?returnUrl=%2Fadmin%2Fusers`
- **AND** the location contains no `query` and no `a%40b.vn`
- **VERIFIED BY**: e2e `referrer hygiene guest admin returnUrl is path only` (`team-frontend/e2e/referrer-hygiene.spec.ts`)

#### Scenario: Referrer hygiene: buildReturnPath drops admin searchParams
- **WHEN** `buildReturnPath("/admin/audit", { target_id: "u-1", actor_id: ["a", "b"] })` is called
- **THEN** it returns `/admin/audit`
- **VERIFIED BY**: `team-frontend/src/features/auth/guardPath.test.ts` `admin returnUrl is path only`

#### Scenario: Referrer hygiene: header-based guard drops the admin query
- **WHEN** the `x-pathname` header is `/admin/cases?status=open` and a param-less guard runs
- **THEN** `redirectToLogin` is called with `/admin/cases`
- **VERIFIED BY**: `guardPath.test.ts` `header guard strips admin query`

#### Scenario: Referrer hygiene: non-admin guard still preserves the query
- **WHEN** `buildReturnPath("/chat", { thread: "t1", tag: ["a", "b"] })` is called
- **THEN** it returns `/chat?thread=t1&tag=a&tag=b`
- **VERIFIED BY**: `guardPath.test.ts` `non-admin query preserved` (existing behaviour, e2e `guard preserves query` in `frontend-login-return`)

#### Scenario: Referrer hygiene: look-alike admin prefix keeps its query
- **WHEN** `buildReturnPath("/administrator", { q: "1" })` is called
- **THEN** it returns `/administrator?q=1`
- **VERIFIED BY**: `guardPath.test.ts` `admin match is segment exact`

#### Scenario: Referrer hygiene: signed-in non-admin still sees the 403 view
- **WHEN** a signed-in buyer opens `/admin/users?query=x`
- **THEN** the response renders "Cần tài khoản Quản trị" with no redirect to login
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/users/page.test.tsx` `non-admin sees 403` (existing, must stay green)
