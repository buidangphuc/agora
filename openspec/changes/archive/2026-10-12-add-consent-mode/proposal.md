# Proposal: Consent banner, Consent Mode v2 and edge consent stamping

initiative: ga4-analytics

## Why
Decree 13/2023 requires explicit consent before personal data is collected. Today `initGTM()` loads
GTM on every page when `NEXT_PUBLIC_GTM_ID` is set, and tracking events carry no proof of consent.
Buyers must choose analytics, marketing and personalisation, and the choice must be enforced at
the browser (GTM), at the edge (stamp on every event) and later in recsys (consumer slices).
Initiative: `plans/ga4-analytics/00-spec.md` (Locked 2026-10-11).

## What changes
- **team-frontend**: a consent banner (analytics / marketing / personalization groups, accept all,
  reject all, save choices, re-open from a footer link) and a consent module that reads and writes
  the first-party cookie `agora_consent` = `v=1&a=0|1&m=0|1&p=0|1&ts=<unix>`.
- **Consent Mode v2, basic mode**: before analytics consent GTM is not loaded and no request goes to
  `googletagmanager.com` / `google-analytics.com`. On analytics grant GTM is injected after the
  dataLayer receives `consent default` (all denied) then `consent update` (analytics_storage
  granted; ad_storage / ad_user_data / ad_personalization follow marketing; personalization_storage
  follows personalization). Revoking consent saves the cookie with `a=0` (the cookie is kept, not removed, as proof of the
  choice), pushes `consent update` denied and stops further pushes; it
  cannot unload an already loaded GTM, so the page reloads GTM-free on next navigation.
- **team-gateway** (`internal/edge/collector.go`): `POST /api/track` parses `agora_consent` from the
  request cookie and stamps `TrackingEvent.consent` (`ConsentState`) on every event. A `consent`
  field in the beacon body is ignored. Missing, malformed or unknown-version cookie stamps
  `version=0`, all false. The edge only parses and stamps; no business logic (AGENTS.md Rule 2).
- Rejected buyers still send first-party beacons (necessary, operational stats); those events carry
  `personalization=false`.
- Hot files shared with `add-ga4-destination`: `destinations/gtm.ts`, `AnalyticsProvider.tsx`; this
  slice lands first.

## Depends on
**Merge order: `ga4-consent-contract` must merge first**; this slice vendors its proto (`TrackingEvent.consent`) and the gateway task cannot compile without it.
`ga4-consent-contract` (platform-core): `TrackingEvent.consent = 26`, `ConsentState{analytics,
marketing, personalization, version}`. This change does not edit the contract. `ga_client_id` (25)
is populated by `add-ga4-destination`, not here.

## NFR
- Banner causes CLS < 0.1 (rendered in a reserved fixed-position layer, no content push) and does
  not block first render or hydration (no render-blocking script, no extra network request).
- Requests to Google hosts before analytics consent: 0.
- Gateway cookie parse adds under 0.1 ms per request and no allocation per event beyond the
  `ConsentState`; `/api/track` p99 stays under the existing 50 ms budget.
- The consent cookie never makes `/api/track` fail: any parse problem degrades to all denied.
- Consent choice persists 180 days (cookie Max-Age); re-prompt afterwards (decision, confirmed 2026-10-12).

## Data classification
| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `agora_consent` cookie (a, m, p, version, ts) | Personal-data-adjacent, proof of consent | Record the buyer's choice, enforce it in browser and edge | Necessary (storing the consent choice itself); no consent needed | 180 days, then re-prompt |
| `TrackingEvent.consent` (ConsentState) | Proof of consent attached to behavioural events | Let recsys and analytics filter by personalisation/analytics consent, audit trail | Legal obligation to prove consent | Same as `tracking_events` retention |
| `ts` in cookie | Timestamp only | Detect stale consent | Necessary | Not forwarded to events |
No email, phone, name or user id is stored in the cookie or in `ConsentState`. The cookie is not
HttpOnly because the banner (JS) must read it; it holds no identifier. Not logged by the gateway.

## Decisions (confirmed 2026-10-12)
- Cookie value uses `&` separators: `v=1&a=0|1&m=0|1&p=0|1&ts=<unix>` (no `;`).
- Max-Age 180 days (15552000 s), then the banner re-prompts. `SameSite=Lax`, `Secure` on https, not HttpOnly (JS must read it), `Path=/`.
- Cookie value capped at 256 bytes; longer is treated as malformed.
- The three flags are independent (e.g. `p=1` with `a=0` is valid).
- Unknown version (anything but `1`) is treated as version 0, all denied.
- Revoke writes the cookie with `a=0`; it never deletes it.
- `ts` must be an unsigned decimal integer; a missing or non-numeric `ts` makes the cookie malformed.

## Threat model
- **Assets**: the buyer's consent decision; integrity of the consent proof on events; the
  guarantee that Google hosts are not contacted without consent.
- **Abuse cases**: (1) a client forges consent in the beacon body to opt itself into
  personalisation, or an attacker poisons another buyer's flag, so the body is never trusted and
  only the cookie is parsed; (2) a crafted cookie (oversized, duplicate keys, non-0/1 values,
  unknown version) crashes the parser or grants consent, so any deviation is treated as
  all-denied; (3) script injection via the cookie value into logs or storage, so values are
  parsed to booleans only and never echoed; (4) consent bypass by a race where GTM loads before
  the cookie is read, so GTM injection awaits a synchronous cookie read.
- **Authz rule**: consent is per browser, cookie-scoped, no authentication needed. The edge
  trusts only the `Cookie` header it received, never the body, never headers such as
  `X-Consent`. A buyer can only change their own browser's cookie. A forged cookie only affects
  the forger's own events (accepted risk: consent is self-asserted).
