@tracking @analytics
Feature: GA4 destination - GTM container, pseudonymous user_id, CSP, debug mode and ga_client_id
  The repository ships a consent-gated GTM container with GA4 tags, the frontend serves a
  pseudonymous HMAC user_id only to consenting authenticated callers, the CSP allows Google hosts
  only when a GTM id is configured, debug_mode is staging-only, and first-party beacons carry the
  GA4 client id only under analytics consent. Google hosts are mocked in e2e.
  Scenarios that need a special build/stack are marked in their steps: the HMAC reference needs
  E2E_GA4_USER_ID_HMAC_KEY (the frontend must run with the same GA4_USER_ID_HMAC_KEY); the
  staging/production debug scenarios need E2E_APP_ENV matching the frontend NEXT_PUBLIC_APP_ENV;
  the unconfigured-GTM scenarios apply only to a frontend built without NEXT_PUBLIC_GTM_ID.

  # ── Versioned GTM container ──────────────────────────────────────────

  Scenario: Container defines event tags for the ecommerce funnel
    When the GTM container JSON is parsed
    Then it has exactly 1 GA4 configuration tag
    And it has one GA4 event tag for each funnel event with a custom-event trigger of the same name

  Scenario: Every tag is consent-gated
    When the GTM container JSON is parsed
    Then every tag requires analytics_storage consent and 0 tags are unconditional

  Scenario: Purchase is not sent from the browser
    When the GTM container JSON is parsed
    Then it contains 0 tags whose event name is purchase

  Scenario: Container contains no key or secret values
    When the GTM container JSON is parsed
    Then it contains 0 occurrences of any value of the HMAC key or the API secret
    And the measurement id is a variable, not a literal

  # ── Pseudonymous GA4 user_id ─────────────────────────────────────────

  Scenario: Logged-in buyer with analytics consent gets a stable HMAC
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer calls the identity endpoint twice with analytics consent
    Then both identity responses are 200 with the same 64-char hex userId equal to the reference HMAC
    And the identity body contains no raw user id, email or name

  Scenario: No analytics consent yields no user_id
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer calls the identity endpoint with a=0 and again with no consent cookie
    And the buyer opens the home page with analytics consent denied
    Then every identity response is 204 with an empty body
    And window.dataLayer holds 0 user_id entries

  Scenario: Anonymous caller yields no user_id
    Given the GA4 identity endpoint is deployed
    When a caller with a=1 and no valid session calls the identity endpoint
    Then every identity response is 204 with an empty body

  Scenario: Forged session cookie yields no user_id
    Given the GA4 identity endpoint is deployed
    And another buyer exists
    When a caller with a=1 presents a session cookie with a future exp, the other buyer as sub and an invalid signature
    Then every identity response is 204 with an empty body

  Scenario: Missing key disables the feature
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer calls the identity endpoint with analytics consent on a deployment without the HMAC key
    Then every identity response is 204 with an empty body
    And the frontend log contains neither the buyer id nor the HMAC key

  Scenario: Caller cannot request another user's id
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    And another buyer exists
    When the buyer calls the identity endpoint with analytics consent naming the other buyer in a userId parameter
    Then the identity response is 200 with the HMAC of the session buyer only

  Scenario: Response is not cacheable
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer calls the identity endpoint with analytics consent
    And a caller with a=1 and no valid session calls the identity endpoint
    Then every identity response has Cache-Control no-store

  Scenario: Identity calls are fast
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer calls the identity endpoint 100 times with analytics consent
    Then the p95 latency of the identity calls is below 50 ms

  Scenario: Consent withdrawal stops user_id
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When the buyer opens the home page with analytics consent granted
    And the buyer withdraws analytics consent through the Cookie settings link
    Then the dataLayer holds a consent update with analytics_storage denied after which no user_id is pushed
    And the next identity call with the browser cookies is 204

  # ── CSP ──────────────────────────────────────────────────────────────

  Scenario: Configured environment allowlists the Google hosts
    When a visitor opens the home page of a deployment with a GTM id while recording CSP violations
    Then the CSP header allows the Google analytics hosts in script-src, connect-src and img-src
    And the CSP header names no other third-party analytics host
    And 0 CSP violation reports were produced

  Scenario: Unconfigured environment has no Google host
    When a visitor opens the home page of a deployment without a GTM id while recording CSP violations
    Then the CSP header is present and contains 0 Google analytics hosts
    And the browser made 0 requests to Google hosts

  Scenario: Unlisted host is blocked
    When a visitor opens the home page while recording CSP violations
    And a script from an unlisted host is injected
    Then the browser blocks it and reports a CSP violation

  # ── One property per environment, debug in staging ───────────────────

  Scenario: Staging events carry debug_mode
    When a buyer opens the home page with analytics consent on a staging deployment
    Then window.dataLayer has an entry with debug_mode true

  Scenario: Production events do not carry debug_mode
    When a buyer opens the home page with analytics consent on a production deployment
    Then window.dataLayer holds ecommerce entries and none has debug_mode true

  Scenario: No IDs in dev, CI and e2e
    Given the GA4 identity endpoint is deployed
    And a buyer is logged in
    When a visitor opens the home page of a deployment without a GTM id and without the HMAC key
    Then no GTM script element exists
    And the identity endpoint returns 204 for an authenticated consenting caller
    And the browser made 0 requests to Google hosts

  # ── ga_client_id in first-party beacons ──────────────────────────────

  Scenario: Beacon carries the client id with analytics consent
    When a visitor with analytics consent and the cookie _ga "GA1.1.1234567890.1700000000" opens the home page
    And the tracking queue idle window elapses
    Then at least one beacon was sent and a beacon has a gaClientId that is "1234567890.1700000000"

  @freshVisitor
  Scenario Outline: No consent means no client id
    When a visitor with the consent cookie "<consent>" and the cookie _ga "GA1.1.1234567890.1700000000" opens the home page
    And the tracking queue idle window elapses
    Then at least one beacon was sent and every beacon has a gaClientId that is empty
    And once the visitor has analytics consent and a valid _ga cookie after a reload the beacons carry its client id

    Examples:
      | consent                          |
      | v=1&a=0&m=0&p=0&ts=1760000000    |
      | absent                           |

  Scenario Outline: Malformed or missing _ga cookie
    When a visitor with analytics consent and the cookie _ga "<ga>" opens the home page
    And the tracking queue idle window elapses
    Then at least one beacon was sent and every beacon has a gaClientId that is empty
    And the page raised no uncaught exception
    And once the visitor has analytics consent and a valid _ga cookie after a reload the beacons carry its client id

    Examples:
      | ga                     |
      | absent                 |
      | garbage                |
      | GA1.1.abc.def          |
      | GA1.1.123456789        |
      | GA2.1.1234567890.17000 |

  @freshVisitor
  Scenario: Batched events share the consent decision
    When a visitor with the consent cookie "absent" and the cookie _ga "GA1.1.1234567890.1700000000" opens the home page
    And the visitor scrolls the page to queue view events
    And the visitor clicks "Accept all" on the consent banner
    And the visitor scrolls the page to queue view events
    And the tracking queue idle window elapses
    Then every beacon enqueued before consent has an empty gaClientId and the later ones carry "1234567890.1700000000"

  Scenario Outline: gaClientId is bounded to 64 characters
    When a visitor with analytics consent and the cookie _ga "GA1.1.<a>.<b>" opens the home page
    And the tracking queue idle window elapses
    Then at least one beacon was sent and every beacon has a gaClientId that is <outcome>
    And once the visitor has analytics consent and a valid _ga cookie after a reload the beacons carry its client id

    Examples:
      | a                                | b                                | outcome |
      | 11111111111111111111111111111111 | 2222222222222222222222222222222  | kept    |
      | 11111111111111111111111111111111 | 22222222222222222222222222222222 | empty   |
