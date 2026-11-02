## MODIFIED Requirements

### Requirement: CreateReview fails closed when team-order cannot answer

When the order verifier returns an error or times out, `CreateReview` SHALL return `Unavailable`, SHALL store no row, and SHALL enqueue no `ReviewCreated` event. A review SHALL never be stored as unverified by `CreateReview`. The verifier SHALL always exist: `UPSTREAM_ORDER_ADDR` is required by the settings in every `ENV`, so a service without a team-order address does not start and no code path stores or skips verification for lack of a client.

#### Scenario: Verifier returns an error
- **WHEN** team-order returns an error for GetOrder during CreateReview
- **THEN** the call returns `Unavailable`, the reviews table has 0 new rows and the outbox has 0 new `ReviewCreated` rows
- **VERIFIED BY**: team-engagement `internal/engagement` `TestCreateReviewVerifierErrorUnavailable`

#### Scenario: Verifier times out
- **WHEN** team-order does not answer within the SDK's fixed 2 s bound
- **THEN** the call returns `Unavailable` within 2 s plus 500 ms and 0 rows are written
- **VERIFIED BY**: team-engagement `internal/upstream` `TestVerifyPurchaseTimeoutIsError`, `internal/engagement` `TestCreateReviewVerifierErrorUnavailable`

#### Scenario: No order upstream configured
- **WHEN** the service starts with `UPSTREAM_ORDER_ADDR` empty
- **THEN** `Load` returns an error that names `UPSTREAM_ORDER_ADDR` and no server is built
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyOrderAddr`

#### Scenario: Upstream not-found is not an outage
- **WHEN** team-order answers `NotFound` for the order id
- **THEN** the call returns `FailedPrecondition` (not `Unavailable`) and 0 rows are written
- **VERIFIED BY**: team-engagement `internal/upstream` `TestVerifyPurchaseNotFoundIsIneligible`
