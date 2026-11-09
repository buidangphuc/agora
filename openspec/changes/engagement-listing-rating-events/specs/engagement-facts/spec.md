## ADDED Requirements

### Requirement: Listing rating changes are published as absolute values

team-engagement SHALL write a `ListingRatingChanged{listing_id, average_rating, review_count, changed_at}` fact, with
record key = listing id, in the same transaction as each write that changes a listing's rating summary: a successful
`CreateReview`, and a `HideReview` that moves a review from visible to hidden. Every such hide SHALL write one event, whether or not the
visible summary changed (values are absolute, so an unchanged summary is harmless). The values SHALL be computed in
that transaction with the rule of `GetListingRatingSummary` (verified purchases that are not hidden), so they equal
what `GetListingRatingSummary` returns once the write has committed; `average_rating` SHALL be 0 when
`review_count` is 0. A call that changes no state (duplicate review, hiding a hidden review, unknown review) SHALL
write no `ListingRatingChanged`. The payload SHALL carry no reviewer, review, seller or moderator identity and no
review text, and the envelope SHALL carry no principal. Writers on one listing SHALL be serialised by
`pg_advisory_xact_lock(hashtext(listing_id))` taken before the aggregate is read. Any future operation that unhides a review SHALL write the same fact. Facts SHALL be published to
`engagement.events` at least once and in `seq` order per listing key, inside the standard `EventEnvelope` with type
`platform.engagement.v1.ListingRatingChanged`.

#### Scenario: A new review publishes the new average and count

- **GIVEN** listing L has one verified review with rating 5
- **WHEN** a buyer with a delivered order reviews L with rating 2
- **THEN** a `ListingRatingChanged` envelope for L with `review_count` 2 and `average_rating` 3.5 appears on `engagement.events`, and `GetListingRatingSummary(L)` returns the same two numbers
- **VERIFIED BY**: team-engagement `internal/store` `TestCreateReviewWritesListingRatingChanged`; e2e `engagement/listing_rating_events.feature`

#### Scenario: Hiding a review lowers the published values

- **GIVEN** listing L has verified reviews with ratings 5 and 1
- **WHEN** an admin hides the rating-1 review
- **THEN** a `ListingRatingChanged` envelope for L with `review_count` 1 and `average_rating` 5.0 appears on `engagement.events`, after the `ReviewHidden` of the same key
- **VERIFIED BY**: team-engagement `internal/store` `TestHideReviewWritesListingRatingChanged`; e2e `engagement/listing_rating_events.feature`

#### Scenario: Hiding the last review publishes zero

- **GIVEN** listing L has exactly one visible verified review
- **WHEN** an admin hides it
- **THEN** a `ListingRatingChanged` envelope for L with `review_count` 0 and `average_rating` 0 appears on `engagement.events`
- **VERIFIED BY**: team-engagement `internal/store` `TestHideLastReviewPublishesZero`

#### Scenario: A call that changes nothing publishes no rating event

- **WHEN** a buyer submits a second review for the same order and listing (rejected as duplicate), and an admin hides an already hidden review
- **THEN** no new `ListingRatingChanged` row exists in the outbox for that listing after either call
- **VERIFIED BY**: team-engagement `internal/store` `TestNoOpWritesPublishNoRatingEvent`

#### Scenario: A rejected review publishes no rating event

- **GIVEN** listing L has one visible verified review and a buyer has no eligible order for L
- **WHEN** that buyer calls `CreateReview` for L
- **THEN** the call is rejected with its existing error, `GetListingRatingSummary(L)` is unchanged, and no `ListingRatingChanged` row is written
- **VERIFIED BY**: team-engagement `internal/rpc` `TestRejectedCreateReviewWritesNoRatingEvent`; e2e `engagement/listing_rating_events.feature`

#### Scenario: An unauthorised hide publishes no rating event

- **GIVEN** listing L has one visible verified review
- **WHEN** a caller who is not an admin calls `HideReview` for it
- **THEN** the call is rejected with its existing error, the review stays visible, and no `ListingRatingChanged` row is written
- **VERIFIED BY**: team-engagement `internal/rpc` `TestUnauthorisedHideReviewWritesNoRatingEvent`; e2e `engagement/listing_rating_events.feature`

#### Scenario: Concurrent reviews converge on the final summary

- **WHEN** two buyers review the same listing at the same time with ratings 5 and 1
- **THEN** the last `ListingRatingChanged` by `seq` for that listing has `review_count` 2 and `average_rating` 3.0
- **VERIFIED BY**: team-engagement `internal/store` `TestConcurrentReviewsConvergeOnFinalRating`

#### Scenario: Concurrent hides converge on the final summary

- **GIVEN** listing L has visible verified reviews with ratings 5, 3 and 1
- **WHEN** an admin hides the rating-3 and rating-1 reviews at the same time
- **THEN** the last `ListingRatingChanged` by `seq` for L has `review_count` 1 and `average_rating` 5.0
- **VERIFIED BY**: team-engagement `internal/store` `TestConcurrentHidesConvergeOnFinalRating`

#### Scenario: A concurrent create and hide converge on the final summary

- **GIVEN** listing L has visible verified reviews with ratings 5 and 1
- **WHEN** a buyer creates a review with rating 3 while an admin hides the rating-1 review
- **THEN** the last `ListingRatingChanged` by `seq` for L has `review_count` 2 and `average_rating` 4.0
- **VERIFIED BY**: team-engagement `internal/store` `TestConcurrentCreateAndHideConvergeOnFinalRating`

#### Scenario: The rating event carries no personal data

- **WHEN** a buyer reviews a listing with the text "rất tốt" and an admin then hides that review with a reason
- **THEN** no `ListingRatingChanged` envelope, live or backfill, carries a principal, and neither its payload nor any other envelope field contains the reviewer id, the admin id, the review id, the seller id, the text "rất tốt" or the reason
- **VERIFIED BY**: team-engagement `internal/events` `TestListingRatingChangedEnvelopeCarriesNoPersonalData`

#### Scenario: A rating event written while Kafka is down is published later

- **WHEN** Redpanda is stopped, a buyer reviews a listing, and Redpanda is started again
- **THEN** one `ListingRatingChanged` envelope for that listing appears on `engagement.events` and its outbox row was never parked
- **VERIFIED BY**: e2e `engagement/listing_rating_events.feature`

### Requirement: Existing reviewed listings are backfilled

The migration that introduces `ListingRatingChanged` SHALL enqueue, in `engagement_outbox_events`, exactly one pending
`ListingRatingChanged` for every listing that has at least one review row, with the values `GetListingRatingSummary`
would return at migration time (0 and 0 for a listing whose reviews are all hidden or unverified). Backfill rows SHALL have the
deterministic `event_id` `md5('listing-rating-backfill:' || listing_id)` formatted as a UUID, have an envelope without caller identity, and
be the only rows the down migration deletes (and only while not published). The migration deploys together with the
new writer binary; a gap between them is closed by the next write on the listing. Re-running the
migration SHALL add no further rows. A listing with no review rows SHALL get no event.

#### Scenario: Each reviewed listing gets one backfill event

- **GIVEN** a database with listing A (reviews 5 and 3, visible), listing B (one review, hidden), listing C (no reviews)
- **WHEN** the migration runs
- **THEN** the outbox holds one pending `ListingRatingChanged` for A with 2 and 4.0, one for B with 0 and 0, and none for C
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0015BackfillsRatingPerReviewedListing`

#### Scenario: Backfill payload decodes with the contract type

- **WHEN** the migration has written the backfill rows for a listing with average 3.6666666666666665
- **THEN** the envelope's payload decodes as `ListingRatingChanged` with that exact `average_rating` and the matching `review_count`
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0015PayloadDecodesAsContractType`

#### Scenario: Backfill is idempotent

- **WHEN** the up migration statements run a second time on the same database
- **THEN** the number of `ListingRatingChanged` outbox rows is unchanged
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0015IsIdempotent`

#### Scenario: Empty database backfills nothing

- **WHEN** the migration runs on a database with no reviews
- **THEN** no `ListingRatingChanged` row is written and the migration succeeds
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0015EmptyReviews`

#### Scenario: Down migration removes only pending backfill rows

- **GIVEN** one backfill row is published, one backfill row is pending, and one live `ListingRatingChanged` row is pending
- **WHEN** the down migration runs
- **THEN** only the pending row whose `event_id` matches the deterministic backfill pattern `md5('listing-rating-backfill:' || aggregate_id)` is gone; the published backfill row and the live pending row remain
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0015DownRemovesOnlyPendingBackfill`

#### Scenario: Backfill reaches the topic

- **WHEN** the service starts after migration 0015 on a database with a reviewed listing
- **THEN** one `ListingRatingChanged` envelope for that listing appears on `engagement.events` with the same numbers as `GetListingRatingSummary`
- **VERIFIED BY**: e2e `engagement/listing_rating_events.feature`
