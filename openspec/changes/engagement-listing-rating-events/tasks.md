## 1. Code track (team-engagement)

- [x] 1.1 Tests first: `internal/store` tests for create, hide, hide-last, no-op, concurrent and no-personal-data; `internal/rpc` rejected-write test; red before the fix (scenarios: new review publishes, hiding lowers, hiding last publishes zero, no-op publishes nothing, rejected create, rejected hide, concurrent reviews converge, concurrent hide converges, create and hide converge, no personal data)
- [x] 1.2 Tests first: `migrations/migration_test.go` `TestMigration0015*` (scenarios: each reviewed listing, payload decodes, idempotent, empty database, down removes only pending backfill)
- [x] 1.3 Shared rating-summary helper used by `GetRatingSummary` and the two writers; `CreateReview` and `HideReview` enqueue `ListingRatingChanged` after their existing fact in the same transaction, only when a row changed (scenarios: new review publishes, hiding lowers, no-op publishes nothing, failed write)
- [x] 1.4 Per-listing `pg_advisory_xact_lock(hashtext(listing_id))` before the aggregate read, in both writers (decided C5); principal-less envelope for the event (C3) (scenarios: concurrent reviews converge, concurrent hide converges, create and hide converge, no personal data)
- [x] 1.5 Fake repository `storetest.FakeReviewRepository` keeps parity for service-level tests (scenarios: new review publishes, hiding lowers)
- [x] 1.6 `migrations/0015_listing_rating_backfill.{up,down}.sql` with deterministic event ids and little-endian double encoding; embed it (scenarios: each reviewed listing, payload decodes, idempotent, empty database, down removes only pending backfill)
- [x] 1.7 Gates: `go vet`, lint, coverage floor of team-engagement, `openspec validate engagement-listing-rating-events --strict`

## 2. e2e track (platform-e2e, written by the e2e author)

- [x] 2.1 `platform-e2e/tests/e2e/features/engagement/listing_rating_events.feature`: create a review and assert the envelope on `engagement.events` equals `GetListingRatingSummary` (scenario: new review publishes)
- [x] 2.2 Same file: admin hides a review and the envelope shows the lowered values after `ReviewHidden` (scenarios: hiding lowers, hiding last publishes zero)
- [x] 2.3 Same file: rejected create (no eligible order) and non-admin hide write no rating event (scenarios: rejected create, rejected hide)
- [x] 2.4 Same file: stop Redpanda, review, restart, one unparked envelope appears (scenario: written while Kafka is down)
- [x] 2.5 Same file: after migration 0015 a seeded reviewed listing's backfill envelope is on the topic (scenario: backfill reaches the topic)
- [x] 2.6 `team-engagement/FEATURES.yaml` entries for the above (e2e author)
