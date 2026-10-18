# edge-route-policy Specification

## Purpose
Defines which RPCs `team-gateway` refuses to route or gates on the admin scope at the edge, how it reports upstream
failures, and which tokens and request headers it accepts, as defence in depth in front of the services' own checks.

## Requirements

### Requirement: Internal-only RPCs are not routed

`team-gateway` SHALL NOT route `ListingService/ReserveStock`, `ListingService/ReleaseStock`,
`ListingService/CommitReservation`, `VoucherService/CommitReservation`, `VoucherService/ReleaseReservation` or
`AuditService/WriteAuditEvent`. For every caller, anonymous or authenticated, they SHALL answer `unimplemented`
(HTTP 501) without contacting the upstream service. `VoucherService/ValidateAndReserve` SHALL stay routed.

#### Scenario: Internal stock and voucher saga RPCs answer 501 at the edge

- **WHEN** a logged-in seller calls `ReserveStock`, `ReleaseStock`, `CommitReservation` or `ReleaseReservation` through the gateway
- **THEN** each call answers HTTP 501 with code `unimplemented` and the listing's stock is unchanged

#### Scenario: Writing an audit event is not exposed at the edge

- **WHEN** a logged-in seller or the seeded admin calls `WriteAuditEvent` through the gateway
- **THEN** the gateway answers HTTP 501 and no audit event is stored

#### Scenario: The listing stock commit RPC answers 501 at the edge

- **WHEN** a logged-in seller, then an anonymous caller, calls `ListingService/CommitReservation` through the gateway with
  the reservation id of a buyer's live order
- **THEN** each call answers HTTP 501 with code `unimplemented` and the listing's stock is unchanged

### Requirement: Admin-only RPCs are gated at the edge

`team-gateway` SHALL require the scope `admin` on `VerificationService/ReviewKyc`, `EngagementService/ResolveDispute`,
`AuditService/QueryAuditLog`, `OrderService/ForceFailSaga` and
`AnalyticsQueryService/GetTrackingQualityReport` and
`AnalyticsQueryService/GetRecommendationPerformance` before forwarding. An anonymous caller SHALL receive
`unauthenticated` (HTTP 401). A verified caller without `admin` SHALL receive `permission_denied` (HTTP 403).

#### Scenario: Anonymous call to an admin-only RPC

- **WHEN** an anonymous client calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 401

#### Scenario: Buyer call to an admin-only RPC

- **WHEN** a logged-in buyer calls `QueryAuditLog`, `ReviewKyc` or `ResolveDispute` through the gateway
- **THEN** the gateway answers HTTP 403

#### Scenario: Admin may query the audit log

- **WHEN** the seeded admin calls `QueryAuditLog` through the gateway
- **THEN** the request succeeds with an events list

#### Scenario: A buyer cannot force-fail their own order

- **WHEN** a buyer calls `ForceFailSaga` through the gateway on their own `Pending` order
- **THEN** the gateway answers HTTP 403 and the order is still `Pending` with its stock still held

### Requirement: Upstream failures are reported without internal detail

When an upstream service fails with an internal or unknown error, the gateway SHALL answer with the message
`internal error`. When the upstream refuses the connection, it SHALL answer `unavailable` (HTTP 503) with the message
`service unavailable`. When the gateway gives up waiting for the upstream, it SHALL answer `deadline_exceeded`
(HTTP 504) with the message `upstream timed out`. Neither response SHALL contain the upstream's error text, addresses or driver messages. The
gateway SHALL log the original error with the request id.

#### Scenario: An unreachable upstream is reported without internal detail

- **WHEN** team-audit is stopped and the seeded admin calls `QueryAuditLog` through the gateway
- **THEN** the gateway answers HTTP 503 `service unavailable` or HTTP 504 `upstream timed out`, and the body contains no host, port, dial or load-balancer text

### Requirement: Tokens must carry an expiry and a subject

The gateway SHALL reject a bearer token that verifies against the JWKS but has no `exp` claim or an empty `sub`. It
SHALL answer `unauthenticated` (HTTP 401).

#### Scenario: A signed token without an expiry is rejected

- **WHEN** a client calls an authenticated RPC with a token signed by the identity key that has no `exp` claim
- **THEN** the gateway answers HTTP 401

#### Scenario: A signed token without a subject is rejected

- **WHEN** a client calls an authenticated RPC with a token signed by the identity key whose `sub` is empty
- **THEN** the gateway answers HTTP 401

### Requirement: Request correlation and idempotency headers are validated

The gateway SHALL accept a client `X-Request-Id` only if it matches `^[A-Za-z0-9._-]{1,64}$`. Otherwise it SHALL
generate a fresh id. Either way it SHALL echo the id it used in the response header `X-Request-Id`. On `CreateOrder`,
an `Idempotency-Key` header that is not a single printable-ASCII value of 1 to 255 bytes SHALL be rejected with
`invalid_argument` (HTTP 400).

#### Scenario: A well-formed request id is echoed

- **WHEN** a client sends `X-Request-Id: e2e-req.42` on an RPC
- **THEN** the response header `X-Request-Id` is `e2e-req.42`

#### Scenario: A malformed request id is replaced

- **WHEN** a client sends an `X-Request-Id` containing spaces and angle brackets
- **THEN** the response header `X-Request-Id` is a different, well-formed id

#### Scenario: A malformed idempotency key is rejected

- **WHEN** a logged-in buyer calls `CreateOrder` with an `Idempotency-Key` longer than 255 bytes
- **THEN** the gateway answers HTTP 400

### Requirement: HideReview is admin-gated and forwarded at the edge

The gateway SHALL forward `EngagementService.HideReview` to team-engagement and SHALL list `EngagementServiceHideReviewProcedure` in `adminProcedures`, so that an anonymous caller is refused with `unauthenticated` and a verified caller without the `admin` scope with `permission_denied` before any upstream call. The forward SHALL be a single attempt (no retry).

#### Scenario: Anonymous caller is refused at the edge
- **WHEN** an anonymous client calls HideReview through the gateway
- **THEN** the gateway returns `unauthenticated` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewAnonymousUnauthenticated`

#### Scenario: Non-admin is refused at the edge
- **WHEN** a buyer without the `admin` scope calls HideReview through the gateway
- **THEN** the gateway returns `permission_denied` and team-engagement receives 0 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewNonAdminPermissionDenied`

#### Scenario: Admin call is forwarded once
- **WHEN** an admin calls HideReview through the gateway and team-engagement returns `Unavailable`
- **THEN** team-engagement receives exactly 1 call and the client gets `unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHideReviewForwardedOnceNoRetry`

#### Scenario: The admin procedure set is pinned
- **WHEN** the pinned-set test compares `adminProcedures` with the expected list
- **THEN** the list contains the existing 7 procedures plus `EngagementServiceHideReviewProcedure` and nothing else
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProceduresPinned`

#### Scenario: HideReview is not left unrouted
- **WHEN** the unrouted guard enumerates EngagementService procedures
- **THEN** `HideReview` is routed by the forwarder
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardCoversHideReview`
