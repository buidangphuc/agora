## ADDED Requirements

### Requirement: EngagementService exposes HideReview

`EngagementService` in `platform/engagement/v1/engagement.proto` SHALL declare `rpc HideReview(HideReviewRequest) returns (HideReviewResponse)`. `HideReviewRequest` SHALL have `string review_id = 1` and `string reason = 2`; `HideReviewResponse` SHALL be an empty message. The RPC comment SHALL state: admin-only (`Unauthenticated` for anonymous callers, `PermissionDenied` for authenticated non-admins), `NotFound` for an unknown review, `InvalidArgument` for an empty review_id or a reason empty or longer than 500 chars after trim, idempotent (already hidden returns OK), `Unavailable` when the audit sink is unavailable with the review left visible, and `Internal` when the hide fails after the audit write. The 1-500 char reason limit is enforced by the server in engagement-review-integrity, not by the wire.

#### Scenario: Service and messages exist in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose `HideReview`, `HideReviewRequest.review_id`, `HideReviewRequest.reason` and an empty `HideReviewResponse`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

#### Scenario: Request field numbers are fixed
- **WHEN** the compiled `HideReviewRequest` descriptor is inspected
- **THEN** `review_id` is field 1 and `reason` is field 2, both of type string, and `HideReviewResponse` has zero fields
- **VERIFIED BY**: Go descriptor/round-trip test `platform-core/packages/proto/platform/engagement/v1/hidereview_roundtrip_test.go` (tasks 2.1)

#### Scenario: Round-trip of a boundary reason
- **WHEN** a `HideReviewRequest` with a 500-char reason and a `HideReviewRequest` with an empty reason are each serialised and parsed
- **THEN** both parse back unchanged (the wire carries validation-free strings; limits are enforced by the server)
- **VERIFIED BY**: Go round-trip test `hidereview_roundtrip_test.go` (tasks 2.1)

#### Scenario: Error and authz contract is documented
- **WHEN** a reader opens the `HideReview` RPC comment in the proto
- **THEN** it names admin-only with `Unauthenticated` for anonymous and `PermissionDenied` for authenticated non-admin callers, `NotFound`, `InvalidArgument` (empty review_id, reason empty or over 500 chars after trim), idempotent OK for an already hidden review, `Unavailable` with the review staying visible, and `Internal` for a failed hide after the audit write
- **VERIFIED BY**: error-code text assertion in `hidereview_roundtrip_test.go` (tasks 2.1) plus comment review (tasks 1.2); behaviour is verified in engagement-review-integrity

#### Scenario: Anonymous and non-admin callers are documented with distinct codes
- **WHEN** a reader checks the contract for an anonymous caller and for an authenticated non-admin caller of `HideReview`
- **THEN** the RPC comment states `Unauthenticated` for anonymous and `PermissionDenied` for the authenticated non-admin, and `HideReviewRequest` has no actor field, so the caller cannot supply or spoof the actor id
- **VERIFIED BY**: descriptor assertion in `hidereview_roundtrip_test.go` that `HideReviewRequest` has exactly the fields `review_id` and `reason`, error-code text assertion in `hidereview_roundtrip_test.go` (tasks 2.1) plus comment review (tasks 1.2); enforcement is verified in engagement-review-integrity

#### Scenario: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0 (`HideReviewRequest`/`HideReviewResponse` follow the `<Rpc>Request`/`<Rpc>Response` convention)
- **VERIFIED BY**: `make -C platform-core lint-proto`

### Requirement: Review rule comments with unchanged wire

The proto SHALL document the review rules in comments only, without changing any field number, name or type: `CreateReviewRequest.order_id` is "required: a COMPLETED order of the caller containing listing_id" (empty is `InvalidArgument`, ineligible `FailedPrecondition`, duplicate for the same order and listing `AlreadyExists`, order service unavailable `Unavailable`); `Review.verified_purchase` is "true for every review created after q2; false = legacy, excluded from ratings"; `review_count` in `GetListingRatingSummaryResponse` and `GetShopRatingSummaryResponse` is "verified, non-hidden reviews".

#### Scenario: Change is backward compatible
- **WHEN** `buf breaking --against` the committed HEAD runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking`

#### Scenario: Existing messages keep their wire shape
- **WHEN** the descriptors of `CreateReviewRequest`, `Review`, `GetListingRatingSummaryResponse` and `GetShopRatingSummaryResponse` are compared before and after the change
- **THEN** every field keeps its name, number and type (`order_id = 4`, `verified_purchase = 11`, `review_count = 3` in both summaries) and no field is added or removed
- **VERIFIED BY**: Go descriptor assertion in `hidereview_roundtrip_test.go` (tasks 2.1) plus `make -C platform-core breaking`

#### Scenario: Wire-breaking edit is rejected
- **WHEN** a field number, name or type of any of the four commented fields is changed or removed
- **THEN** `buf breaking` fails and the check does not pass
- **VERIFIED BY**: `make -C platform-core breaking` plus the descriptor assertion in `hidereview_roundtrip_test.go` (tasks 2.1, 1.4)

#### Scenario: Rule comments are present
- **WHEN** a reader opens the four commented fields in the proto
- **THEN** each carries the exact rule text above
- **VERIFIED BY**: four-string comment assertion (reads the proto file and checks each rule text) in `hidereview_roundtrip_test.go` (tasks 2.1)

### Requirement: Proto copies are identical across consumers

The updated `engagement.proto` SHALL be copied byte-identical into `team-engagement/proto/platform/engagement/v1/`, `team-gateway/proto/platform/engagement/v1/` and `team-frontend/proto/platform/engagement/v1/`. Consumers SHALL NOT edit their copy.

#### Scenario: Copies match the source
- **WHEN** each consumer copy is compared with `platform-core/packages/proto/platform/engagement/v1/engagement.proto`
- **THEN** all three comparisons report identical content (exit 0)
- **VERIFIED BY**: `cmp` of the three copies against the source (tasks 2.3)

#### Scenario: Consumer copies lint
- **WHEN** `buf lint` runs inside each of `team-engagement/proto`, `team-gateway/proto` and `team-frontend/proto`
- **THEN** each exits 0 with the new RPC present
- **VERIFIED BY**: `buf lint` per consumer proto directory (tasks 2.3)

#### Scenario: Drifted copy is rejected
- **WHEN** any consumer copy differs from the source by even one byte
- **THEN** the `cmp` check exits non-zero and the contract slice is not done
- **VERIFIED BY**: `cmp` of the three copies against the source (tasks 2.3)
