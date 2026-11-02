## MODIFIED Requirements

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
