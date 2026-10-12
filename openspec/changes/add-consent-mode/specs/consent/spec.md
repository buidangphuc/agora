# Specification: Consent banner and Consent Mode v2

## ADDED Requirements

### Requirement: Consent cookie format
The frontend SHALL persist the buyer's choice in the first-party cookie `agora_consent` with the value `v=1&a=<0|1>&m=<0|1>&p=<0|1>&ts=<unix seconds>` (`&` separators, no `;`), attributes `Path=/; SameSite=Lax; Max-Age=15552000` plus `Secure` on https and no `HttpOnly`, SHALL treat the flags as independent, SHALL cap the value at 256 bytes, and SHALL treat an absent, malformed (including missing or non-numeric `ts`) or unknown-version cookie as all groups denied with version 0.

#### Scenario: Accept all writes the cookie
- **WHEN** a buyer clicks "Accept all" on the banner at unix time T
- **THEN** `document.cookie` contains `agora_consent=v=1&a=1&m=1&p=1&ts=T` within 100 ms and the banner is removed from the DOM

#### Scenario: Reject all writes the cookie
- **WHEN** a buyer clicks "Reject all"
- **THEN** the cookie value is `v=1&a=0&m=0&p=0&ts=<unix>` and 0 requests go to `googletagmanager.com` or `google-analytics.com`

#### Scenario: Granular choice is stored per group
- **WHEN** a buyer enables only personalization and clicks "Save choices"
- **THEN** the cookie value is `v=1&a=0&m=0&p=1&ts=<unix>`

#### Scenario: Unknown version is treated as denied
- **WHEN** `agora_consent` holds `v=2&a=1&m=1&p=1&ts=1760000000`
- **THEN** the consent module returns all three groups false with version 0 and the banner is shown

#### Scenario: Missing fields are treated as denied
- **WHEN** `agora_consent` holds `a=1&m=1`
- **THEN** the consent module returns all three groups false with version 0 and the banner is shown

#### Scenario: Non-0/1 flag is treated as denied
- **WHEN** `agora_consent` holds `v=1&a=2&m=0&p=0&ts=1760000000`
- **THEN** the consent module returns all three groups false with version 0 and the banner is shown

#### Scenario: Non-numeric or missing ts is treated as denied
- **WHEN** `agora_consent` holds `v=1&a=1&m=1&p=1&ts=abc` or `v=1&a=1&m=1&p=1`
- **THEN** the consent module returns all three groups false with version 0 and the banner is shown

#### Scenario: Flags are independent
- **WHEN** `agora_consent` holds `v=1&a=0&m=0&p=1&ts=1760000000`
- **THEN** the consent module returns personalization true, analytics false and marketing false with version 1

#### Scenario: Oversized cookie is treated as denied
- **WHEN** `agora_consent` holds a valid-looking value longer than 256 bytes
- **THEN** the consent module returns all three groups false with version 0

#### Scenario: Boundary at the 180-day lifetime
- **WHEN** the cookie is written
- **THEN** its Max-Age equals 15552000 seconds (180 days) and the cookie has `SameSite=Lax`, no `HttpOnly`, `Secure` on https, and a browser without the cookie sees the banner again

### Requirement: Banner on first visit without layout shift
The frontend SHALL show the consent banner on a visit with no valid `agora_consent` cookie, SHALL render it in a fixed-position layer that does not move page content, and SHALL NOT block first render.

#### Scenario: First visit shows banner and contacts no Google host
- **WHEN** a buyer with no cookie loads the home page with `NEXT_PUBLIC_GTM_ID=GTM-TEST`
- **THEN** the banner is visible with three groups (analytics, marketing, personalization), the DOM has 0 `googletagmanager.com` scripts and the browser issues 0 requests to `googletagmanager.com` and `google-analytics.com`

#### Scenario: Banner does not shift layout
- **WHEN** the banner mounts on a cold load
- **THEN** the page's Cumulative Layout Shift attributed to the banner is below 0.1

#### Scenario: Returning buyer sees no banner
- **WHEN** a buyer loads a page with a valid `agora_consent` cookie
- **THEN** the banner is not rendered and a footer "Cookie settings" link re-opens it

#### Scenario: Banner is keyboard operable
- **WHEN** the banner opens
- **THEN** every control is reachable by Tab, the container has `role="dialog"` with an accessible name, and Escape does not record a choice

### Requirement: Consent Mode v2 basic gating of GTM
The frontend SHALL NOT inject the GTM script until `agora_consent` has `a=1`; WHEN analytics is granted it SHALL push `consent default` (all denied) before GTM loads and `consent update` with `analytics_storage=granted` after the grant, with `ad_storage`, `ad_user_data` and `ad_personalization` equal to the marketing choice and `personalization_storage` equal to the personalization choice.

#### Scenario: Accept analytics loads GTM and updates consent
- **WHEN** a buyer grants analytics on a page with `NEXT_PUBLIC_GTM_ID=GTM-TEST`
- **THEN** `window.dataLayer` contains a `consent default` entry with every signal `denied` followed by a `consent update` entry with `analytics_storage=granted`, and exactly 1 GTM script is injected after them

#### Scenario: Marketing and personalization map to signals
- **WHEN** a buyer grants analytics and marketing but not personalization
- **THEN** the `consent update` entry has `ad_storage`, `ad_user_data`, `ad_personalization` equal `granted` and `personalization_storage` equal `denied`

#### Scenario: Returning consented buyer loads GTM once
- **WHEN** a buyer with `a=1` loads a page
- **THEN** `consent default` then `consent update` are in `window.dataLayer` before the single GTM script is injected, with 0 duplicate scripts after client navigation

#### Scenario: Rejected analytics never loads GTM
- **WHEN** the cookie has `a=0`
- **THEN** no GTM script is injected on any route and ecommerce `dataLayer` pushes are not forwarded to a Google host

#### Scenario: Empty GTM id keeps GTM off even with consent
- **WHEN** `NEXT_PUBLIC_GTM_ID` is empty and the buyer grants analytics
- **THEN** no script is injected, the cookie is still written and the e2e stack issues 0 requests to Google hosts

#### Scenario: Revoking analytics stops pushes
- **WHEN** a buyer who granted analytics later saves `a=0`
- **THEN** the cookie is saved (not removed) with value `v=1&a=0&...&ts=<unix>`, a `consent update` with `analytics_storage=denied` is pushed, and no GTM script is injected on the next page load

### Requirement: First-party beacons are independent of consent
The frontend SHALL keep sending first-party `/api/track` beacons for visitors who reject all groups, and SHALL NOT include any consent value in the beacon body.

#### Scenario: Rejected visitor still sends a beacon
- **WHEN** a visitor who rejected all views a listing
- **THEN** one `POST /api/track` is sent and its body has no `consent` key

#### Scenario: Consent change applies to later beacons only
- **WHEN** a buyer grants personalization at time T
- **THEN** beacons sent before T carry no personalization consent and beacons sent after T are stamped `personalization=true` by the edge
