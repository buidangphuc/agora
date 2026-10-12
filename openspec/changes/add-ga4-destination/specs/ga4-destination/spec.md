## ADDED Requirements

### Requirement: Versioned GTM container with GA4 tags

The repository SHALL contain `team-frontend/analytics/gtm-container.json`, a GTM container export
defining one GA4 configuration tag and GA4 event tags triggered by dataLayer ecommerce events. Every
tag SHALL require `analytics_storage=granted` (consent settings) and no tag SHALL send `purchase`.

#### Scenario: Container defines event tags for the ecommerce funnel

- **WHEN** the container JSON is parsed
- **THEN** it has exactly 1 GA4 configuration tag and one GA4 event tag per event in
  {view_item_list, select_item, view_item, add_to_cart, remove_from_cart, view_cart, begin_checkout,
  add_shipping_info, add_payment_info}, each with a custom-event trigger of the same name

#### Scenario: Every tag is consent-gated

- **WHEN** the container JSON is parsed
- **THEN** every tag requires `analytics_storage` consent and 0 tags are unconditional

#### Scenario: Purchase is not sent from the browser

- **WHEN** the container JSON is parsed
- **THEN** it contains 0 tags whose event name is `purchase` (server-side Measurement Protocol is the only purchase source)

#### Scenario: Container contains no key or secret values

- **WHEN** the container JSON is scanned
- **THEN** it contains 0 occurrences of any value of `GA4_USER_ID_HMAC_KEY` or `GA4_API_SECRET`, and the measurement id is a variable, not a literal

### Requirement: Pseudonymous GA4 user_id

The frontend server SHALL compute the GA4 `user_id` as lowercase hex HMAC-SHA256 of the session
principal id keyed by `GA4_USER_ID_HMAC_KEY`, and `GET /api/analytics/identity` SHALL return it only
to an authenticated caller whose `agora_consent` cookie has `a=1`. The client SHALL push it to the
dataLayer as `{ user_id }` only after the analytics `consent update`. The frontend server SHALL take the
principal id only from the gateway edge endpoint `GET /api/session/principal`, which verifies the bearer
(JWKS signature, expiry, revocation) and returns 200 `{"principalId": "<sub>"}` with `Cache-Control: no-store`,
or 401 with an empty body for a missing, invalid, expired or revoked token; a 401 or any gateway error makes
the identity endpoint answer 204 (Clarifications R1 Q1).

#### Scenario: Logged-in buyer with analytics consent gets a stable HMAC

- **WHEN** an authenticated buyer with `agora_consent` `a=1` calls `GET /api/analytics/identity` twice
- **THEN** both responses are 200 with the same 64-char hex `userId` equal to HMAC-SHA256(user id, key), and the body contains no raw user id, email or name

#### Scenario: No analytics consent yields no user_id

- **WHEN** an authenticated buyer with `a=0` or no `agora_consent` cookie calls the endpoint
- **THEN** the response is 204 with an empty body and the dataLayer receives 0 `user_id` pushes

#### Scenario: Anonymous caller yields no user_id

- **WHEN** a caller with `a=1` but no valid session calls the endpoint
- **THEN** the response is 204 and no HMAC is computed

#### Scenario: Forged session cookie yields no user_id

- **WHEN** a caller with `a=1` presents a `session` cookie holding a well-formed JWT with a future `exp`, a `sub` naming another user and an invalid signature, and calls `GET /api/analytics/identity`
- **THEN** the response is 204 with an empty body

#### Scenario: Gateway principal endpoint accepts only verified tokens

- **WHEN** `GET /api/session/principal` is called with no bearer, a bad signature, an expired token or a revoked token, and then with a valid token
- **THEN** the first four answer 401 with an empty body, and the valid token answers 200 `{"principalId"}` equal to its `sub` with `Cache-Control: no-store`
- **VERIFIED BY**: `team-gateway/internal/edge/session_principal_test.go` "TestSessionPrincipal_OnlyVerifiedTokens" (Clarifications R1 Q1)

#### Scenario: Missing key disables the feature

- **WHEN** `GA4_USER_ID_HMAC_KEY` is unset or empty
- **THEN** the endpoint returns 204 for every caller and no error is logged containing the key or user id

#### Scenario: Caller cannot request another user's id

- **WHEN** the request carries a `userId` query parameter or body naming another user
- **THEN** the parameter is ignored and the returned value, if any, is the HMAC of the session principal only

#### Scenario: Response is not cacheable

- **WHEN** the endpoint answers 200 or 204
- **THEN** the response has `Cache-Control: no-store`

#### Scenario: Identity calls are fast

- **WHEN** 100 identity calls run against the endpoint
- **THEN** p95 latency < 50 ms

#### Scenario: Consent withdrawal stops user_id

- **WHEN** consent is withdrawn after `user_id` was pushed, i.e. the dataLayer receives a `consent update` with `analytics_storage=denied`, or the `agora_consent` cookie becomes `a=0`
- **THEN** no further `user_id` push occurs in that page session and the next identity call returns 204

### Requirement: CSP allows Google hosts only when GA4 is configured

The frontend SHALL emit a `Content-Security-Policy` header whose `script-src`, `connect-src` and
`img-src` include `https://www.googletagmanager.com`, `https://*.google-analytics.com`, `https://*.analytics.google.com` and `https://region1.google-analytics.com` (deviation from 00-spec's two hosts: these are real GA4 endpoints) if and only
if `NEXT_PUBLIC_GTM_ID` is non-empty, and SHALL include no other third-party analytics host.

#### Scenario: Configured environment allowlists the Google hosts

- **WHEN** a page is served with `NEXT_PUBLIC_GTM_ID=GTM-XXXX`
- **THEN** the CSP header contains `googletagmanager.com` in `script-src` and `*.google-analytics.com`, `*.analytics.google.com` and `region1.google-analytics.com` in `connect-src`, and a GTM load in the browser produces 0 CSP violation reports

#### Scenario: Unconfigured environment has no Google host

- **WHEN** a page is served with `NEXT_PUBLIC_GTM_ID` empty (dev/CI/e2e)
- **THEN** the CSP header contains 0 occurrences of `googletagmanager.com`, `google-analytics.com` or `analytics.google.com` and the browser makes 0 requests to either host

#### Scenario: Unlisted host is blocked

- **WHEN** a script from a host outside the allowlist is injected
- **THEN** the browser blocks it and reports a CSP violation

### Requirement: One GA4 property per environment, debug in staging

The frontend SHALL take the GTM id from `NEXT_PUBLIC_GTM_ID` per environment (one GA4 property per
environment) and SHALL set `debug_mode=true` on GA4 events only when `NEXT_PUBLIC_APP_ENV=staging`.

#### Scenario: Staging events carry debug_mode

- **WHEN** a GA4 event is pushed with `NEXT_PUBLIC_APP_ENV=staging` and analytics consent granted
- **THEN** the dataLayer config entry has `debug_mode: true`

#### Scenario: Production events do not carry debug_mode

- **WHEN** the same push happens with `NEXT_PUBLIC_APP_ENV=production`
- **THEN** the dataLayer has no `debug_mode` key set to true

#### Scenario: No IDs in dev, CI and e2e

- **WHEN** the app starts with `NEXT_PUBLIC_GTM_ID` and `GA4_USER_ID_HMAC_KEY` unset
- **THEN** no GTM script element exists, `/api/analytics/identity` returns 204, and 0 requests go to Google

### Requirement: ga_client_id in first-party beacons

First-party `/api/track` beacons SHALL include `gaClientId` parsed from the `_ga` cookie only while
analytics consent is granted; otherwise `gaClientId` SHALL be omitted or empty.

#### Scenario: Beacon carries the client id with analytics consent

- **WHEN** `_ga=GA1.1.1234567890.1700000000` is set, analytics consent is granted and a view event is queued
- **THEN** the flushed beacon has `gaClientId` equal to `1234567890.1700000000` (client-id part, no `GA1.1.` prefix)

#### Scenario: No consent means no client id

- **WHEN** `_ga` is set but analytics consent is denied or the `agora_consent` cookie is absent
- **THEN** every flushed beacon has an empty `gaClientId` and the `_ga` cookie is not read

#### Scenario: Malformed or missing _ga cookie

- **WHEN** `_ga` is absent or does not match `GA1.<n>.<a>.<b>`
- **THEN** `gaClientId` is empty and no exception reaches the caller

#### Scenario: Batched events share the consent decision

- **WHEN** consent flips from denied to granted between two queued events of one batch
- **THEN** each beacon uses the consent state at its own enqueue time, so the earlier event keeps an empty `gaClientId`

#### Scenario: gaClientId is bounded to 64 characters

- **WHEN** the parsed client-id part exceeds 64 characters
- **THEN** `gaClientId` is empty
