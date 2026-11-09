# engagement-facts Specification

## Purpose
Defines which engagement state changes (favourites, follows, review ratings) team-engagement publishes as facts
through its transactional outbox, and how team-analytics stores them and derives the current state.

## Requirements

### Requirement: Engagement state changes are published as facts

team-engagement SHALL write a fact in the same transaction as each of these state changes:
- `FavoriteAdded` for `AddFavorite` and `FavoriteRemoved` for `RemoveFavorite`, both carrying user and listing;
- `SellerFollowed` for `FollowSeller` and `SellerUnfollowed` for `UnfollowSeller`, both carrying user and seller;
- `ReviewCreated` for `CreateReview`, carrying user, listing, seller and rating, and no review text.

A call that changes no state SHALL write no fact, for example adding an existing favourite or removing a missing one.
Facts SHALL be published in write (`seq`) order for each record key (two concurrent transactions on one key can commit in the opposite order) to `engagement.events` inside the standard `EventEnvelope`;
facts with different keys carry no order guarantee between them. The envelope's principal SHALL be the caller and its
`type` SHALL be the payload's full name. The record key SHALL be the listing id, or the seller id for follows. A fact
SHALL be published at least once, even if Kafka is down when it is written, and a broker outage SHALL NOT park it.

#### Scenario: Favouriting a listing publishes a fact

- **WHEN** a logged-in buyer adds a listing to their favourites through the gateway
- **THEN** one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayPublishesPerKeyInOrder`; e2e `engagement/engagement_facts.feature`

#### Scenario: Removing a favourite publishes a removal

- **WHEN** the buyer then removes that favourite
- **THEN** a `FavoriteRemoved` envelope for that buyer and listing follows it on `engagement.events`, after the `FavoriteAdded` of the same key
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayPublishesPerKeyInOrder`; e2e `engagement/engagement_facts.feature`

#### Scenario: A repeated favourite publishes nothing new

- **WHEN** a buyer adds the same listing to their favourites twice
- **THEN** exactly one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`
- **VERIFIED BY**: team-engagement `internal/store` `TestRepeatedFavouriteWritesOneFact`; e2e `engagement/engagement_facts.feature`

#### Scenario: A review publishes its rating without its text

- **WHEN** a buyer with a delivered order reviews the listing with rating 4 and the text "rất tốt"
- **THEN** a `ReviewCreated` envelope with rating 4, the listing and its seller appears on `engagement.events`, and the
  payload does not contain "rất tốt"
- **VERIFIED BY**: team-engagement `internal/events` `TestFactRowBuildsEnvelope`; e2e `engagement/engagement_facts.feature`

#### Scenario: A fact written while Kafka is down is published later

- **WHEN** Redpanda is stopped, a buyer follows a seller, and Redpanda is started again
- **THEN** one `SellerFollowed` envelope for that buyer and seller appears on `engagement.events` and its outbox row was never parked
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayTransientErrorsNeverPark`; e2e `engagement/engagement_facts.feature`

#### Scenario: Facts of different keys carry no mutual order

- **WHEN** a buyer favourites listing `L1` and follows seller `S1` in two transactions
- **THEN** both envelopes appear on `engagement.events` exactly once, and only the order within key `L1` and within key `S1` is asserted
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayOrderIsPerKeyOnly`

### Requirement: The warehouse stores engagement facts and their current state

team-analytics SHALL consume `engagement.events` into `engagement_facts`, keeping one row per `event_id`, with `fact`,
`user_id`, `listing_id`, `seller_id`, `rating`, `occurred_at` and `ingested_at`. An undecodable record SHALL go to
`engagement.events.analytics.dlq` and SHALL NOT stop consumption. The view `favorites_current` SHALL list the
(user, listing) pairs whose latest favourite fact is an add. The view `follows_current` SHALL list the (user, seller)
pairs whose latest follow fact is a follow.

#### Scenario: A favourite that was removed is not current

- **WHEN** a buyer adds two listings to their favourites and then removes the first one
- **THEN** `engagement_facts` holds three rows for the buyer, and `favorites_current` lists only the second listing for
  the buyer

#### Scenario: A follow appears in the current follows

- **WHEN** a buyer follows a seller
- **THEN** `follows_current` lists that buyer and seller

#### Scenario: A malformed engagement record is dead-lettered

- **WHEN** a malformed record is produced to `engagement.events`, followed by a buyer favouriting a listing
- **THEN** the malformed record appears on `engagement.events.analytics.dlq`, and the favourite still reaches
  `engagement_facts`

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
