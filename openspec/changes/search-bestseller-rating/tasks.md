## 1. Code track: team-search (tests first)

- [x] 1.1 Unit tests for the sales and rating handlers: apply, older and equal `changed_at` ignored, pending for unknown id, deleted id ignored, malformed events return an error (BR-1, BR-2, BR-3, BR-4, BR-4a, BR-5, BR-7, BR-7a, BR-8); verify red first
- [x] 1.2 Integration tests on the index: mapping, pending doc invisible, merge on upsert, recreate after delete, update keeps projected values, `sortClause`, `min_rating` clause, ratings aggregation ignoring its own filter, purge of expired pending docs (BR-5, BR-6, BR-8a, BR-9, BR-10, BR-13, BR-14, BR-15, BR-18, BR-18a, pending-doc scenarios)
- [x] 1.3 rpc tests: `min_rating` 0..5 accepted, outside rejected, saved-search run with out-of-range value rejected, hit fields mapped, zero values (BR-12, BR-17, BR-18b, BR-19, BR-20)
- [x] 1.4 Additive mapping fields `sold_count` (long), `rating_avg` (double), `review_count` (long), plus the two version fields and `pending_at` (date, epoch_millis) in `internal/index/opensearch.go`, applied at indexer startup; every pending write sets `pending_at`, the listing upsert clears it and keeps the projected values; events later than a tombstone's `version` become pending values on it (BR-5, BR-6, BR-8, BR-8a)
- [x] 1.5 Handlers beside `NewListingEventHandler` in `internal/events/` with validation and the `changed_at` guard (BR-1 to BR-4a, BR-7, BR-7a, BR-8)
- [x] 1.6 One consumer client per topic on group `KAFKA_CONSUMER_GROUP`, each with `kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())` (as `team-engagement/internal/events/listing.go:30-31` does), `search-indexer.<topic>.dlq` topics created by redpanda-init (C17), defaults for `KAFKA_ORDER_TOPIC` and `KAFKA_ENGAGEMENT_TOPIC`; a missing topic delays consumption and does not crash startup; no `*_ENABLED` flag (BR-7, BR-7a)
- [x] 1.7 `SORT_BY_BEST_SELLING` in `sortClause`, lexical leg only in every mode (BR-9, BR-10, BR-11)
- [x] 1.8 Accept `min_rating` 0..5 in `internal/rpc/search.go`, filter on `rating_avg` for all modes and saved search, ratings facet floors 4 and 3 ignoring its own filter (BR-13 to BR-18b)
- [x] 1.9 Map the three fields onto `SearchHit` (BR-19, BR-20)
- [x] 1.10 Extend the sweep to delete documents with `pending_at` older than `TOMBSTONE_TTL` and no `status`, never live documents (pending-doc scenarios in `search-read-model-deletes`)
- [x] 1.11 Rebase on `search-attribution` for the shared file `internal/rpc/search.go` (and `internal/index/opensearch.go`); consume regenerated stubs

## 2. E2E track (platform-e2e; the e2e-author owns FEATURES.yaml)

- [x] 2.1 Rewrite `search/search_query_correctness.feature` scenarios "A minimum-rating search is rejected" and "The ratings facet is empty" into accepted-search and filled-facet scenarios, add out-of-range rejection (BR-13, BR-17, BR-18)
- [x] 2.2 New `search/search_best_selling_rating.feature` publishing `ListingSalesChanged` and `ListingRatingChanged` to Kafka: sort order, ties, boundary, pending merge, malformed and empty-id events on DLQ, deleted listing, facet ignoring own filter, draft hidden (BR-1 to BR-20)
- [x] 2.3 `search/search_read_model_deletes.feature`: "An expired pending document is purged" (pending-doc scenarios)
- [x] 2.4 Rewrite, not delete, the team-search FEATURES entries `search.srm-a-minimum-rating-search-is-rejected` and `search.srm-the-ratings-facet-is-empty` (title, acceptance, `covered_by`); add entries for the new scenarios
- [x] 2.5 Run on a feature env after `order-listing-sales` and `engagement-listing-rating-events` are merged (BR-1, BR-2, BR-5)

## 3. Verification

- [x] 3.1 `openspec validate search-bestseller-rating --strict`, `spec_lint.py`, team-search checks, features check
- [ ] 3.2 Measure `SearchListings` p95 for `sort=best_selling` and `min_rating` against baseline, and for searches without them (NFR, non-gating)
