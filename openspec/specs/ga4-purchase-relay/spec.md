# ga4-purchase-relay Specification

## Purpose
TBD - created by archiving change add-ga4-purchase-relay. Update Purpose after archive.

## Requirements

### Requirement: Client id mapping
The relay SHALL keep, per `buyer_id`, the `ga_client_id` of the most recent `analytics.events` TrackingEvent whose `ga_client_id` is non-empty and whose `consent.analytics` is true, and SHALL NOT record events with empty `ga_client_id`, denied analytics consent or empty `buyer_id`. WHEN an event for a buyer has `consent.analytics=false`, THE SYSTEM SHALL delete that buyer's mapping. Mappings expire after a TTL of 30 days and are stored in a relay-owned table via the team-analytics warehouse adapter.

#### Scenario: Latest consented client id wins
- **WHEN** buyer `b1` emits events with `ga_client_id` `111.1` then `222.2`, both with `consent.analytics=true`
- **THEN** the mapping for `b1` is `222.2`

#### Scenario: Denied consent deletes the mapping
- **WHEN** `b1` has a mapping `222.2` and then an event for `b1` arrives with `consent.analytics=false`
- **THEN** the mapping for `b1` no longer exists, the event is counted in `ga4_relay_mapping_skipped_total{reason="no_consent"}` and a later `OrderPaidEvent` for `b1` sends 0 requests

#### Scenario: Anonymous event is ignored
- **WHEN** an event has an empty `buyer_id`
- **THEN** no mapping is written

#### Scenario: Mapping expires
- **WHEN** 30 days pass with no new consented event for `b1`
- **THEN** a later `OrderPaidEvent` for `b1` finds no mapping and sends nothing
- **VERIFIED BY**: `team-analytics/internal/ga4relay/mapping_test.go` "TestMappingExpiresAfter30Days" (Clarifications R1 Q1)

### Requirement: One purchase per paid order
WHEN the relay receives an `OrderPaidEvent` for a buyer with a mapping, THE SYSTEM SHALL send exactly one MP `purchase` with `client_id` = mapped id, `transaction_id` = `order_id`, `currency`, `value` = `total_amount` and one item per `OrderLineItemFact` with `item_id` = `listing_id` (`listing_id:variant_id` when `variant_id` is non-empty), `quantity`, `price` = `unit_price` and `currency`. `value` SHALL be the order `total_amount` and MAY differ from the sum of items. The relay SHALL NOT send `buyer_id`, GA4 `user_id` or any contact data (`user_id` is out of scope). WHEN `paid_at` is nil, THE SYSTEM SHALL still send the purchase without `timestamp_micros` and count `ga4_relay_paid_at_missing_total`.

#### Scenario: Paid order is relayed
- **WHEN** an `OrderPaidEvent` for order `o1` (2 line items, total 500000 VND, buyer `b1` mapped to `222.2`) is consumed
- **THEN** the GA4 mock receives exactly 1 request whose body has `client_id=222.2`, one `purchase` event with `transaction_id=o1`, `currency=VND`, `value=500000` and 2 items, each with `item_id`, `quantity`, `price`

#### Scenario: Payload carries no personal data
- **WHEN** the request for order `o1` is captured
- **THEN** the body and URL contain none of `buyer_id`, `user_id`, `email`, `phone`, and the URL carries `measurement_id` and `api_secret` only

#### Scenario: Item ids and value mapping
- **WHEN** an `OrderPaidEvent` has `total_amount=500000` and items `(listing l1, variant v1, qty 2, unit_price 200000)` and `(listing l2, no variant, qty 1, unit_price 90000)`
- **THEN** the items are `item_id=l1:v1, quantity=2, price=200000` and `item_id=l2, quantity=1, price=90000`, and `value=500000` (not 490000)

#### Scenario: Paid_at is nil
- **WHEN** an `OrderPaidEvent` has no `paid_at`
- **THEN** exactly 1 request is sent without `timestamp_micros` and `ga4_relay_paid_at_missing_total` increments by 1

#### Scenario: Latency budget
- **WHEN** `OrderPaidEvent` with `paid_at` = T is consumed while the GA4 mock is healthy
- **THEN** the MP request is received by T + 5 minutes

#### Scenario: Other envelope types are skipped
- **WHEN** an `order.events` record has a type other than `platform.order.v1.OrderPaidEvent`
- **THEN** no request is sent and the offset is committed

#### Scenario: Large order is capped
- **WHEN** an `OrderPaidEvent` has 250 line items
- **THEN** the request contains exactly 200 items and `value` still equals `total_amount`

#### Scenario: Malformed order event is rejected
- **WHEN** an `OrderPaidEvent` has an empty `order_id` or an undecodable payload
- **THEN** no request is sent, the record goes to the relay DLQ and `ga4_relay_failed_total{reason="invalid"}` increments by 1

#### Scenario: Another buyer's client id is never used
- **WHEN** buyer `b1` and buyer `b2` both have mappings and `b2` pays order `o2`
- **THEN** the request for `o2` carries `b2`'s client id and not `b1`'s

### Requirement: Idempotent delivery
The relay SHALL record each successfully sent `order_id` in a durable ledger before committing the Kafka offset and SHALL NOT send for an `order_id` already in the ledger.

#### Scenario: Kafka redelivery
- **WHEN** the same `OrderPaidEvent` for `o1` is delivered 3 times
- **THEN** the GA4 mock receives exactly 1 request for `o1`

#### Scenario: Restart after send
- **WHEN** the relay restarts after sending `o1` but before the offset commit, and `o1` is redelivered
- **THEN** no second request for `o1` is sent

#### Scenario: Concurrent duplicates
- **WHEN** two `OrderPaidEvent` records for `o1` are processed concurrently
- **THEN** the GA4 mock receives exactly 1 request for `o1`

#### Scenario: Failed send is not recorded as sent
- **WHEN** the send for `o3` fails permanently and `o3` is redelivered after the GA4 mock recovers
- **THEN** the relay sends exactly 1 request for `o3`

### Requirement: Retry and failure handling
The relay SHALL make at most 5 attempts in total (1 initial + 4 retries) for network errors, HTTP 5xx and 429, with exponential backoff (base 1 s, factor 2, cap 30 s), SHALL NOT retry other 4xx, SHALL NOT write the ledger entry until a 2xx is returned, and WHEN a ledger or mapping-store write fails on all 5 delivery attempts of a record, THE SYSTEM SHALL dead-letter that record to `GA4_RELAY_DLQ_TOPIC` and count each failed attempt in `ga4_relay_failed_total{reason="store"}`, so a purchase or client-id mapping lost to a warehouse outage is recoverable by replaying the dead-letter topic. Because `Client.Send` blocks up to 40 s on one record (5 attempts of at most 5 s each plus the gaps 1, 2, 4 and 8 s) and the SDK blocks group rebalances while a poll is handled, the relay consumer SHALL set `MaxPollRecords` to 1 so that `MaxPollRecords x 40 s` stays under the 60 s group rebalance timeout.

#### Scenario: Transient failure then success
- **WHEN** the GA4 mock returns 503 twice then 204 for `o1`
- **THEN** exactly 3 requests are made and `Client.Send` returns nil
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_TransientFailureThenSuccess`

#### Scenario: Permanent client error is not retried
- **WHEN** the GA4 mock returns 400 (or any 4xx other than 429, e.g. 403) for `o1`
- **THEN** exactly 1 request is made, `ga4_relay_failed_total{reason="4xx"}` increments by 1, the record goes to the relay DLQ `analytics.ga4-relay.dlq` and the offset is committed without a ledger entry
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_PermanentClientErrorNotRetried`
- **VERIFIED BY**: team-analytics `internal/app` `TestRejectedOrderIsDeadLetteredAndNextOrderSent`

#### Scenario: Rate limited is retried
- **WHEN** the GA4 mock returns 429 once then 204 for `o1`
- **THEN** exactly 2 requests are made, the second after at least 1 s, and the ledger contains `o1`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_RateLimitedIsRetried`

#### Scenario: Server error backoff schedule
- **WHEN** the GA4 mock returns 500 for every attempt for `o1`
- **THEN** exactly 5 requests are made with gaps of at least 1 s, 2 s, 4 s and 8 s, and none exceeds 30 s
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_ServerErrorBackoffSchedule`

#### Scenario: Retries exhausted
- **WHEN** the GA4 mock returns 503 for all 5 attempts (1 initial + 4 retries) for `o1`
- **THEN** `ga4_relay_failed_total{reason="exhausted"}` increments by 1, the record goes to the relay DLQ topic and consumption continues with the next record
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_RetriesExhausted`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleExhaustedRetriesIsPermanent`

#### Scenario: Request timeout boundary
- **WHEN** the GA4 mock does not answer within 5 seconds
- **THEN** the attempt is counted as a network error and retried
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_TimeoutCountedAsNetworkErrorAndRetried_RequestTimeoutBoundary`

#### Scenario: A GA4 outage does not exceed the rebalance timeout
- **WHEN** the relay consumer options are built from the default settings
- **THEN** `MaxPollRecords` is 1, and `MaxPollRecords x (5 x RequestTimeout + 1 s + 2 s + 4 s + 8 s)` is 40 s, strictly under the 60 s group rebalance timeout
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestRelayConsumerOptionsFitRebalanceBudget`

#### Scenario: A store outage beyond the delivery attempts is dead-lettered and counted
- **WHEN** an `OrderPaidEvent` for `o4` is produced to `order.events` on a Redpanda broker while every ledger lookup fails
- **THEN** the record appears on `analytics.ga4-relay.dlq` within 30 s with its original key and the SDK `dlq-*` headers, `ga4_relay_failed_total{reason="store"}` increases by 5, the GA4 stub receives 0 requests, and after the ledger recovers replaying that dead-letter record sends exactly 1 request for `o4`
- **VERIFIED BY**: team-analytics `internal/app` `TestStoreOutageIsDeadLetteredAfterFiveAttemptsAndReplaySends`

### Requirement: No consented client id, no send
IF the buyer has no mapping, THEN THE SYSTEM SHALL send nothing for that order and SHALL count it in `ga4_relay_skipped_total{reason="no_client_id"}`.

#### Scenario: Buyer without mapping
- **WHEN** an `OrderPaidEvent` arrives for buyer `b2` who has no consented client id
- **THEN** the GA4 mock receives 0 requests, the skip counter increments by 1 and the offset is committed

#### Scenario: Consent revoked before payment
- **WHEN** buyer `b1`'s latest event before PAID has `consent.analytics=false`
- **THEN** the mapping for `b1` is deleted and no request is sent for the order

#### Scenario: Unconsented buyer is not sent under another identity
- **WHEN** an `OrderPaidEvent` arrives for a buyer whose only events have `consent.analytics=false`
- **THEN** the GA4 mock receives 0 requests

### Requirement: No inbound surface and restricted order.events writers
The relay SHALL expose no inbound endpoint other than the gRPC health service `grpc.health.v1.Health` (overall status and the `liveness` service), and only the order service's producer credentials SHALL be able to write `order.events`, so that an `OrderPaidEvent` cannot be forged by another principal.

#### Scenario: No inbound endpoint beyond probes
- **WHEN** an unauthenticated gRPC client calls any method other than `grpc.health.v1.Health/Check` and `Watch` on the relay's listening port
- **THEN** the call returns `Unimplemented` and 0 requests reach the GA4 mock
- **VERIFIED BY**: team-analytics `internal/app` `TestRelayServesOnlyHealth`

#### Scenario: Plain HTTP is not served
- **WHEN** an HTTP/1.1 `GET /healthz` is sent to the relay's listening port
- **THEN** the connection yields no HTTP 200 response
- **VERIFIED BY**: team-analytics `internal/app` `TestRelayDoesNotServeHTTPProbes`

#### Scenario: Forged OrderPaidEvent from another principal is rejected
- **WHEN** a principal other than the order service's producer credentials tries to produce an `OrderPaidEvent` to `order.events`
- **THEN** the broker denies the write with an authorization error and the GA4 mock receives 0 requests
- **VERIFIED BY**: `platform-gitops/platform/infra/redpanda.yaml` ACL granting `order.events` write only to the order service principal, checked at review; not testable on the local plaintext Redpanda (Clarifications R1 Q1)

### Requirement: Disabled without secret
WHILE `ENV` is `local` or `test` and `GA4_API_SECRET` or `GA4_MEASUREMENT_ID` is unset, THE SYSTEM SHALL make no outbound HTTP call to Google, SHALL NOT subscribe to Kafka topics, SHALL NOT open the relay's DuckDB file, SHALL report SERVING on `grpc.health.v1.Health` overall and `liveness`, and SHALL log one `ga4 relay disabled` line; WHILE `ENV` is strict, an unset `GA4_API_SECRET` or `GA4_MEASUREMENT_ID` SHALL fail `config.Load`, so a deployed relay is never disabled.

#### Scenario: Secret unset
- **WHEN** the relay starts with `ENV=local` and `GA4_API_SECRET` empty
- **THEN** it logs `ga4 relay disabled`, reports SERVING on the overall health service, joins no consumer group and makes 0 outbound requests
- **VERIFIED BY**: team-analytics `internal/app` `TestRelaySecretUnsetIsDisabledAndServing`

#### Scenario: Secret never logged
- **WHEN** a send fails with a network error while the secret is set
- **THEN** no log line contains the secret value
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestClient_SecretNeverLogged`

#### Scenario: No Google traffic in e2e
- **WHEN** the e2e stack runs with the relay pointed at the GA4 mock endpoint
- **THEN** 0 requests leave the stack to `google-analytics.com`
- **VERIFIED BY**: platform-e2e `features/tracking/ga4_purchase_relay.feature` "No Google traffic in e2e"

### Requirement: Browser no longer sends purchase to GA4
The frontend `map.ts` SHALL NOT map the internal `purchase` event to a browser GA4/dataLayer `purchase`, and the first-party `/api/track` beacon `purchase` SHALL still be sent.

#### Scenario: dataLayer has no purchase
- **WHEN** the order confirmation page tracks a purchase for order `o1`
- **THEN** `window.dataLayer` contains no entry with `event=purchase` and the `/api/track` request contains one `purchase` event with `transaction_id=o1`

#### Scenario: Other events unchanged
- **WHEN** a `begin_checkout` is tracked
- **THEN** `window.dataLayer` contains one `begin_checkout` entry as before

#### Scenario: Warehouse purchase still recorded
- **WHEN** the beacon `purchase` for `o1` is consumed by the warehouse consumer
- **THEN** `tracking_events` has one `purchase` row with `transaction_id=o1`

#### Scenario: Purchase with missing transaction id is not mapped to GA4
- **WHEN** the order confirmation tracks a purchase with an empty `transaction_id`
- **THEN** `window.dataLayer` contains no `purchase` entry and the beacon is still sent
- **VERIFIED BY**: `team-frontend/src/lib/analytics/map.test.ts` "does not map purchase with empty transaction_id to GA4" (Clarifications R1 Q1)
