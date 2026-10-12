@tracking @analytics
Feature: GA4 server-side purchase relay
  A relay consumes OrderPaidEvent from order.events, resolves the buyer's last consented GA4 client
  id from analytics.events and posts one Measurement Protocol purchase per paid order to a GA4 MP
  mock (the relay is pointed at the mock; no request leaves the stack to Google).
  Stack: the relay container (GA4_RELAY_CONTAINER, default agora-ga4-relay-svc) runs with
  GA4_ENDPOINT=http://host.docker.internal:<GA4_MOCK_PORT>/mp/collect and the secret set;
  scenarios that pre-seed mappings wait GA4_MAPPING_SETTLE_S for the pipeline to catch up.
  The 30 day "Mapping expires" scenario cannot be driven end to end (no clock control).

  Background:
    Given the GA4 MP mock is listening and the relay targets it

  # ── Client id mapping ────────────────────────────────────────────────

  Scenario: Latest consented client id wins
    Given buyer "b1" emits consented events with GA client ids "111.1" then "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::2:200000,l2::1:90000"
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the request for order "o1" has client id "222.2"

  Scenario: Denied consent deletes the mapping
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And buyer "b1" then emits an event with analytics consent denied
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives 0 requests for order "o1"
    And the relay commits past the record by relaying the next order of a mapped buyer

  Scenario: Anonymous event is ignored
    Given an anonymous event with GA client id "333.3" and consent granted is emitted
    When order "o1" for buyer "b2" is paid with total 100000 and items "l1::1:100000"
    Then the GA4 mock receives 0 requests for order "o1"
    And no request to the GA4 mock carries client id "333.3"

  # ── One purchase per paid order ──────────────────────────────────────

  Scenario: Paid order is relayed
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::2:200000,l2::1:90000"
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the request for order "o1" has client id "222.2"
    And the request for order "o1" has one purchase event with transaction id "o1", currency "VND", value 500000 and 2 items each with item id, quantity and price

  Scenario: Payload carries no personal data
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::2:200000,l2::1:90000"
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the request for order "o1" body and URL contain none of "buyer_id, user_id, email, phone"
    And the URL of the request for order "o1" carries measurement_id and api_secret only

  Scenario: Item ids and value mapping
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1:v1:2:200000,l2::1:90000"
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the items of the request for order "o1" are "l1:v1:2:200000,l2:-:1:90000" and its value is 500000

  Scenario: Paid_at is nil
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000" and no paid_at
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the request for order "o1" has no timestamp_micros

  Scenario: Latency budget
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives its request for order "o1" within 300 seconds of paid_at

  Scenario: Other envelope types are skipped
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When an order.events record of type "platform.order.v1.OrderCancelled" for order "o1" and buyer "b1" is produced
    Then the GA4 mock receives 0 requests for order "o1"
    And the relay commits past the record by relaying the next order of a mapped buyer

  Scenario: Large order is capped
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 7000000 and 250 distinct line items
    Then the GA4 mock receives exactly 1 request for order "o1"
    And the request for order "o1" has exactly 200 items and value 7000000

  Scenario: Malformed order event is rejected
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When an OrderPaidEvent with an empty order id for buyer "b1" is produced
    And an order.events record of type "platform.order.v1.OrderPaidEvent" with an undecodable payload for buyer "b1" is produced
    Then no request to the GA4 mock carries client id "222.2"
    And both malformed records reach the relay DLQ

  Scenario: Another buyer's client id is never used
    Given buyer "b1" emits consented events with GA client ids "111.1"
    And buyer "b2" emits consented events with GA client ids "222.2"
    When order "o2" for buyer "b2" is paid with total 90000 and items "l2::1:90000"
    Then the GA4 mock receives exactly 1 request for order "o2"
    And the request for order "o2" has client id "222.2"

  # ── Idempotent delivery ──────────────────────────────────────────────

  Scenario: Kafka redelivery
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When the same OrderPaidEvent for order "o1" and buyer "b1" is delivered 3 times
    Then the GA4 mock receives exactly 1 request for order "o1"

  Scenario: Restart after send
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    And the GA4 mock has received 1 request for order "o1"
    And the relay is killed and restarted
    And the same OrderPaidEvent for order "o1" and buyer "b1" is delivered 1 times
    Then the GA4 mock receives exactly 1 request for order "o1"

  Scenario: Concurrent duplicates
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When two OrderPaidEvent records for order "o1" and buyer "b1" are produced concurrently
    Then the GA4 mock receives exactly 1 request for order "o1"

  Scenario: Failed send is not recorded as sent
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o3" with "400"
    When order "o3" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    And the GA4 mock has received 1 request for order "o3"
    And the GA4 mock recovers for order "o3"
    And the same OrderPaidEvent for order "o3" and buyer "b1" is delivered 1 times
    Then the GA4 mock receives exactly 2 requests for order "o3"
    And the last request for order "o3" was answered 2xx

  # ── Retry and failure handling ───────────────────────────────────────

  Scenario: Transient failure then success
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "503,503,204"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 3 requests for order "o1"
    And redelivering order "o1" for buyer "b1" sends nothing more

  Scenario Outline: Permanent client error is not retried
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "<status>"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 1 request for order "o1"
    And order "o1" reaches the relay DLQ

    Examples:
      | status |
      | 400    |
      | 403    |

  Scenario: Rate limited is retried
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "429,204"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 2 requests for order "o1"
    And the gaps between requests for order "o1" are at least "1" seconds

  Scenario: Server error backoff schedule
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "500,500,500,500,500"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 5 requests for order "o1"
    And the gaps between requests for order "o1" are at least "1,2,4,8" seconds and none exceeds 30 seconds

  Scenario: Retries exhausted
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "503,503,503,503,503"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 5 requests for order "o1"
    And order "o1" reaches the relay DLQ
    And the relay commits past the record by relaying the next order of a mapped buyer

  Scenario: Request timeout boundary
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "hang:8,204"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 2 requests for order "o1"
    And the second request for order "o1" arrived about 5 seconds after the first, not after the mock answered

  # ── No consented client id, no send ──────────────────────────────────

  Scenario: Buyer without mapping
    When order "o1" for buyer "b2" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives 0 requests for order "o1"
    And the relay commits past the record by relaying the next order of a mapped buyer

  Scenario: Consent revoked before payment
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And buyer "b1" then emits an event with analytics consent denied
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives 0 requests for order "o1"

  Scenario: Unconsented buyer is not sent under another identity
    Given buyer "b3" emits only events with analytics consent denied
    When order "o1" for buyer "b3" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives 0 requests for order "o1"

  # ── No inbound surface and restricted order.events writers ───────────

  Scenario: No inbound endpoint beyond probes
    When an unauthenticated client requests the paths "/, /send, /metrics-admin" on the relay's listening port
    Then every response is 404 or the connection is refused
    And the GA4 mock received 0 requests from those calls

  Scenario: Forged OrderPaidEvent from another principal is rejected
    Given buyer "b1" emits consented events with GA client ids "222.2"
    When a principal other than the order service tries to produce an OrderPaidEvent for order "o1" and buyer "b1"
    Then the broker denies the write with an authorization error
    And the GA4 mock receives 0 requests for order "o1"

  # ── Disabled without secret ──────────────────────────────────────────

  Scenario: Secret unset
    When the relay is started with an empty GA4_API_SECRET
    Then it logs "ga4 relay disabled" once, answers its readiness probe, joins no consumer group and the GA4 mock received 0 requests

  Scenario: Secret never logged
    Given buyer "b1" emits consented events with GA client ids "222.2"
    And the GA4 mock answers order "o1" with "drop,204"
    When order "o1" for buyer "b1" is paid with total 500000 and items "l1::1:500000"
    Then the GA4 mock receives exactly 2 requests for order "o1"
    And no relay log line contains the secret value

  Scenario: No Google traffic in e2e
    Then the relay is configured with the mock endpoint and not a google-analytics.com host
    And the relay logs mention no google-analytics.com host
