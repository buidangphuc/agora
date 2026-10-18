# review-integrity Specification

## Purpose
Only a buyer with a completed order can review, once per order and listing, and ratings count only verified, non-hidden reviews.

## Requirements

### Requirement: A review requires the caller's completed order

`EngagementService.CreateReview` SHALL require a non-empty `order_id` and SHALL store a review only when team-order reports that order as belonging to the caller, in status `ORDER_STATUS_COMPLETED`, and containing `listing_id`. The stored review SHALL have `verified_purchase=true` and the order's `seller_id`. An ineligible order SHALL return `FailedPrecondition` with one fixed message regardless of which condition failed.

#### Scenario: Completed order is reviewed
- **WHEN** a buyer calls CreateReview with rating 5 for a listing contained in the buyer's COMPLETED order
- **THEN** the response review has `verified_purchase=true` and ListReviews for the listing returns it
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewVerifiedStoresVerified`

#### Scenario: Empty order id is rejected
- **WHEN** a buyer calls CreateReview with an empty `order_id`
- **THEN** the call returns `InvalidArgument` and no review row exists
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReviewEmptyOrderInvalidArgument`

#### Scenario: Ineligible order returns one message
- **WHEN** a buyer calls CreateReview with an order that is not COMPLETED, belongs to another buyer, does not contain the listing, or does not exist
- **THEN** each call returns `FailedPrecondition` with the identical message text and no review row is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReviewIneligibleOrderSingleMessage`

#### Scenario: Anonymous caller cannot review
- **WHEN** an anonymous caller calls CreateReview
- **THEN** the call returns `Unauthenticated` and the order verifier is not called
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReviewAnonymousUnauthenticated`

#### Scenario: Rating boundary values
- **WHEN** a buyer with an eligible order calls CreateReview with rating 0, then 6, then 1, then (on a second listing in the order) 5
- **THEN** ratings 0 and 6 return `InvalidArgument` with no row, and ratings 1 and 5 are stored
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewRatingBounds`

### Requirement: CreateReview fails closed when team-order cannot answer

When the order verifier is nil, returns an error, or times out, `CreateReview` SHALL return `Unavailable`, SHALL store no row, and SHALL enqueue no `ReviewCreated` event. A review SHALL never be stored as unverified by `CreateReview`.

#### Scenario: Verifier returns an error
- **WHEN** team-order returns an error for GetOrder during CreateReview
- **THEN** the call returns `Unavailable`, the reviews table has 0 new rows and the outbox has 0 new `ReviewCreated` rows
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewVerifierErrorUnavailable`

#### Scenario: Verifier times out
- **WHEN** team-order does not answer within `UPSTREAM_CALL_TIMEOUT_SECONDS`
- **THEN** the call returns `Unavailable` within that timeout plus 500 ms and 0 rows are written
- **VERIFIED BY**: team-engagement `internal/upstream` `TestVerifyPurchaseTimeoutIsError`, `internal/service` `TestCreateReviewVerifierErrorUnavailable`

#### Scenario: No order upstream configured
- **WHEN** CreateReview runs with `UPSTREAM_ORDER_ADDR` unset (nil verifier)
- **THEN** the call returns `Unavailable` and 0 rows are written
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewNilVerifierUnavailable`

#### Scenario: Upstream not-found is not an outage
- **WHEN** team-order answers `NotFound` for the order id
- **THEN** the call returns `FailedPrecondition` (not `Unavailable`) and 0 rows are written
- **VERIFIED BY**: team-engagement `internal/upstream` `TestVerifyPurchaseNotFoundIsIneligible`

### Requirement: One verified review per order and listing

The system SHALL enforce at most one verified review per (`order_id`, `listing_id`) with a partial unique index `WHERE verified_purchase`. A second CreateReview for the same pair SHALL return `AlreadyExists`. Migration `0011_review_integrity` SHALL, before creating the index, set `verified_purchase=false` on every duplicate verified row except the earliest per pair.

#### Scenario: Duplicate review is refused
- **WHEN** a buyer calls CreateReview twice for the same order and listing
- **THEN** the second call returns `AlreadyExists` and exactly 1 row exists for the pair
- **VERIFIED BY**: team-engagement `internal/repository` `TestPGCreateReviewDuplicateOrderListing`

#### Scenario: Concurrent duplicates yield one row
- **WHEN** 10 CreateReview calls for the same order and listing run in parallel
- **THEN** exactly 1 succeeds, 9 return `AlreadyExists`, and exactly 1 `ReviewCreated` outbox row exists
- **VERIFIED BY**: team-engagement `internal/repository` `TestPGCreateReviewConcurrentDuplicates`

#### Scenario: Same order, different listing is allowed
- **WHEN** a buyer reviews two different listings from one COMPLETED order
- **THEN** both calls succeed
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewSameOrderTwoListings`

#### Scenario: Migration demotes later duplicates
- **WHEN** migration 0011 runs on data with 3 verified rows for one (order, listing) pair created at t1 < t2 < t3, and 1 verified row for another pair
- **THEN** the t1 row and the other pair's row keep `verified_purchase=true`, the t2 and t3 rows become false, no row is deleted, and the migration completes without error
- **VERIFIED BY**: team-engagement `internal/repository` `TestMigration0011DemotesDuplicateVerified`

#### Scenario: Migration leaves unverified rows alone
- **WHEN** migration 0011 runs on unverified rows sharing an (order, listing) pair, including empty `order_id`
- **THEN** all those rows are unchanged and no unique violation occurs
- **VERIFIED BY**: team-engagement `internal/repository` `TestMigration0011IgnoresUnverified`

#### Scenario: Down migration removes the new schema
- **WHEN** migration 0011 is applied and then rolled back
- **THEN** the index and the columns `hidden_at`, `hidden_by`, `hide_reason` no longer exist and no row is lost
- **VERIFIED BY**: team-engagement `internal/repository` `TestMigration0011Down`

### Requirement: Ratings count verified, non-hidden reviews only

`GetListingRatingSummary` and `GetShopRatingSummary` SHALL compute `average_rating`, `review_count` and the star breakdown only from reviews with `verified_purchase=true` and `hidden_at IS NULL`. `ListReviews` SHALL return every non-hidden review (verified or legacy unverified), SHALL never return a hidden review, and its `page.total` SHALL count all non-hidden reviews matching the rating filter. The Postgres and in-memory repositories SHALL return identical results.

#### Scenario: Listing summary ignores unverified reviews
- **WHEN** a listing has verified reviews of 5 and 3 stars and one legacy unverified 1-star review
- **THEN** GetListingRatingSummary returns `average_rating` 4.0, `review_count` 2 and a breakdown of one 5-star and one 3-star
- **VERIFIED BY**: team-engagement `internal/repository` `TestPGRatingSummaryVerifiedOnly`

#### Scenario: Shop summary applies the same rule
- **WHEN** a seller's listings hold verified reviews of 5 and 3 stars, one hidden verified 1-star review and one unverified 1-star review
- **THEN** GetShopRatingSummary returns `average_rating` 4.0 and `review_count` 2
- **VERIFIED BY**: team-engagement `internal/repository` `TestPGShopSummaryVerifiedNonHidden`

#### Scenario: Listing with no verified reviews
- **WHEN** a listing without any verified review has only an unverified review
- **THEN** GetListingRatingSummary returns `review_count` 0 and `average_rating` 0
- **VERIFIED BY**: team-engagement `internal/repository` `TestRatingSummaryNoVerifiedIsZero`

#### Scenario: ListReviews shows legacy reviews
- **WHEN** ListReviews runs for the listing with 2 verified and 1 unverified non-hidden review
- **THEN** the response holds 3 reviews, `page.total` is 3, and the unverified one has `verified_purchase=false`
- **VERIFIED BY**: team-engagement `internal/handler` `TestListReviewsIncludesUnverifiedExcludesHidden`

#### Scenario: Hidden review is excluded from the list and the total
- **WHEN** a listing has reviews A (5 stars), B (5 stars, hidden) and C (3 stars), and ListReviews runs unfiltered and with rating filter 5
- **THEN** the unfiltered response holds A and C with `page.total` 2, and the filter-5 response holds only A with `page.total` 1
- **VERIFIED BY**: team-engagement `internal/repository` `TestPGListReviewsExcludesHidden`

#### Scenario: In-memory repository matches Postgres
- **WHEN** the same fixture (verified, unverified and hidden reviews across two listings of one seller) runs on both repositories
- **THEN** list, total, listing summary and shop summary results are equal
- **VERIFIED BY**: team-engagement `internal/repository` `TestReviewRepoParity`

### Requirement: ReviewCreated is emitted only for stored verified reviews

`CreateReview` SHALL enqueue `ReviewCreated` in the same transaction as the stored review, and SHALL enqueue nothing when the call is refused for any reason.

#### Scenario: Stored review emits one event
- **WHEN** a buyer creates a verified review
- **THEN** exactly one `ReviewCreated` outbox row exists with that review id, listing id, seller id and rating
- **VERIFIED BY**: team-engagement `internal/repository` `TestOutboxReviewCreated`

#### Scenario: Refused review emits nothing
- **WHEN** CreateReview is refused with `Unavailable`, `FailedPrecondition`, `InvalidArgument` or `AlreadyExists`
- **THEN** the outbox row count does not change
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReviewRefusedWritesNoOutbox`

### Requirement: API-level journeys prove the rule end to end

The e2e suite SHALL cover the review rules through the gateway using a shared completed-order fixture.

#### Scenario: Review from a completed order, end to end
- **WHEN** a buyer completes an order through the gateway and reviews the listing with that order id
- **THEN** ListReviews shows the review with `verified_purchase=true` and the listing summary `review_count` is 1
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A completed order can be reviewed once"

#### Scenario: Second review for the same order is refused, end to end
- **WHEN** the same buyer submits a second review for the same order and listing through the gateway
- **THEN** the gateway returns `already_exists`
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A completed order can be reviewed once"

#### Scenario: Review without an order is refused, end to end
- **WHEN** a buyer submits a review with an empty order id through the gateway
- **THEN** the gateway returns `invalid_argument`
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A review without an order is refused"

#### Scenario: Review of an order that is not completed is refused, end to end
- **WHEN** a buyer submits a review with an order still in a non-COMPLETED status
- **THEN** the gateway returns `failed_precondition`
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "An order that is not completed cannot be reviewed"

#### Scenario: Summary reflects verified reviews, end to end
- **WHEN** two buyers review one listing with 5 and 3 stars from their own COMPLETED orders through the gateway
- **THEN** GetListingRatingSummary through the gateway returns `average_rating` 4.0 and `review_count` 2
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "Stars count verified reviews only"

#### Scenario: Legacy unverified review is listed but excluded from stars, end to end
- **WHEN** a listing has 1 verified review of 5 stars and 1 legacy unverified review of 1 star (seeded by direct SQL)
- **THEN** ListReviews through the gateway returns both with `page.total` 2, and the listing summary returns `average_rating` 5.0 and `review_count` 1
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A legacy unverified review is listed but not counted"
