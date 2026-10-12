# Proposal: GA4 destination (GTM container, pseudonymous user_id, CSP, ga_client_id in beacons)

initiative: ga4-analytics

## Why

Marketing needs acquisition and ecommerce funnels in GA4. The dataLayer already carries GA4
ecommerce events (`add-ga4-gtm-datalayer`) and `add-consent-mode` gates GTM on analytics consent,
but no GA4 tags exist, there is no stable pseudonymous user id, no CSP lets Google hosts load, and
first-party beacons do not carry the GA client id needed to join GA4 sessions with first-party events.

## What changes

- Versioned GTM container export `team-frontend/analytics/gtm-container.json`: one GA4 configuration
  tag plus GA4 event tags driven by dataLayer ecommerce events (`view_item_list`, `select_item`,
  `view_item`, `add_to_cart`, `remove_from_cart`, `view_cart`, `begin_checkout`, `add_shipping_info`,
  `add_payment_info`). No `purchase` tag: purchase is sent server-side by `add-ga4-purchase-relay`.
  All tags require `analytics_storage=granted`.
- GA4 `user_id` = HMAC-SHA256(user id) hex, computed server-side with `GA4_USER_ID_HMAC_KEY`, served
  by a new Next route handler `GET /api/analytics/identity` only when the `agora_consent` cookie has
  `a=1` and the buyer is logged in; the client pushes it to the dataLayer (`{ user_id }`) after the
  consent update. Raw user id never reaches the browser or Google.
- Content-Security-Policy (built in `next.config.mjs` headers) allows `googletagmanager.com`,
  `*.google-analytics.com`, `*.analytics.google.com` and `region1.google-analytics.com` only in environments where `NEXT_PUBLIC_GTM_ID` is non-empty.
- One GA4 property per environment: GTM id / measurement id differ per env via config, not code.
  Staging sets `debug_mode=true` on GA4 events; production does not.
- Dev/CI/e2e: all IDs empty, no Google host in CSP, no request to Google.
- First-party beacons (`WireTrackBeacon`) gain `gaClientId`, parsed from the `_ga` cookie
  (`GA1.<n>.<a>.<b>` -> `ga_client_id` = the client-id part `<a>.<b>`, without the `GA1.<n>.` prefix), sent only while analytics consent is granted, empty
  otherwise; bounded to 64 characters (longer value -> empty). Maps to proto `TrackingEvent.ga_client_id = 25` (contract from `ga4-consent-contract`).
  The edge consent stamp (`consent = 26`) is owned by `add-consent-mode`; the body never carries it.

Depends on `add-consent-mode` (cookie `agora_consent` = `v=1&a=0|1&m=0|1&p=0|1&ts=<unix>`, GTM
loaded only after analytics consent, dataLayer `consent update`). Hot files edited:
`src/lib/analytics/destinations/gtm.ts`, `src/features/tracking/AnalyticsProvider.tsx`,
`src/lib/analytics/map.ts`.

## NFR

- Before analytics consent: 0 requests to googletagmanager.com / google-analytics.com.
- `/api/analytics/identity` p95 < 50 ms over 100 identity calls (one HMAC, no backend call); no caching (`Cache-Control: no-store`).
- CSP deviation from 00-spec (two hosts): GA4 also uses `*.analytics.google.com` and `region1.google-analytics.com` (real collection endpoints), so they are allowlisted; no other host.
- Reading `_ga` and the identity fetch must never throw into UI nor block render; failure = no user_id.
- Feature disabled (no GTM, no user_id, empty `gaClientId`) when `NEXT_PUBLIC_GTM_ID` or
  `GA4_USER_ID_HMAC_KEY` unset.
- Secret `GA4_USER_ID_HMAC_KEY` is server-only (never `NEXT_PUBLIC_`), provisioned via ExternalSecrets.

## Data classification

| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| GA4 `user_id` (HMAC-SHA256 hex of user id) | pseudonymous personal data | cross-device GA4 reporting | analytics consent (`a=1`), checked server-side per request | Google GA4 property retention (set to 14 months, event-level); not stored by Agora |
| `ga_client_id` (from `_ga`) in beacons | pseudonymous identifier | join GA4 export sessions to first-party events (attribution) | analytics consent (`a=1`); empty otherwise | follows `tracking_events` retention |
| `_ga` cookie | identifier set by Google | GA4 client id | analytics consent; GTM not loaded before | 2 years (Google default), cleared by consent withdrawal handled in `add-consent-mode` |
| dataLayer ecommerce payload | behavioural | GA4 funnels | analytics consent | GA4 retention as above |

No email, phone, name or raw user id in any GA4 payload or dataLayer push.

## Threat model

- Assets: HMAC key, user id pseudonym, `_ga` client id.
- Abuse: (1) HMAC key leak enables re-identification by dictionary of ids -> server-only env, never
  logged, rotation = new pseudonyms (documented). (2) Identity endpoint called without consent or
  with forged cookie -> server re-reads `agora_consent`, returns 204 unless `a=1`; a forged `a=1`
  only yields the caller's own HMAC. (3) Unauthenticated caller -> 204, never a value. (4) Another
  user's id requested -> endpoint takes no parameter, derives id from the session cookie only.
  (5) CSP too wide -> only two Google hosts, only when configured. (6) Body-supplied `gaClientId`
  without consent -> client omits it; the edge (add-consent-mode) stamps consent so downstream can
  filter.
- Authz rule: user_id derived only from the caller's own session and only with analytics consent.
