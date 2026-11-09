## Context

team-search is a read-model with no database. `listing.events` upserts documents under a version guard
(`occurred_at` in nanoseconds). Stock already uses a separate `stock_version` partial update. This change adds two
more event sources with the same pattern. The exact envelope type strings for the new messages are assumed to follow
the existing convention (`platform.order.v1.ListingSalesChanged`, `platform.engagement.v1.ListingRatingChanged`).

## Decisions

- **Absolute values, per-field version.** `sold_count` carries `sales_version = changed_at` ns, and
  `rating_avg` + `review_count` carry `rating_version = changed_at` ns. A write applies only when its version is
  greater than the stored one (equal versions are an idempotent no-op). This is the same shape as `stock_version`.
- **Pending values (D2).** A partial update for an id with no document is a `doc_as_upsert` that stores only
  `id`, the values and their versions, with no `status`. Search already requires `status = published`, so such a
  doc never appears in results, totals or facets. The listing upsert merges the stored sales and rating fields
  and versions into the new document instead of overwriting them (as it already keeps the earliest `created_at`).
  Pending docs older than `TOMBSTONE_TTL` (default 336h) are removed by the existing sweep (decision C6); see the delta on `search-read-model-deletes`.
- **Deleted listings.** The tombstone already stores the deletion time: `version` is the delete event's `occurred_at` in
  nanoseconds (`team-search/internal/index/opensearch.go:110` and the `delete` branch of `writeScript` at `:315`); no new
  field is added. A sales or rating event for a tombstoned listing whose `changed_at` (ns) is not later than that
  `version` is ignored. One whose `changed_at` is later is kept as pending values on the tombstone document (status stays
  `deleted`, so it is invisible) and merged by the next listing upsert that is newer than the tombstone. If the
  tombstone is purged first (`tombstoned_at` older than `TOMBSTONE_TTL`), those values go with it.
- **Sort.** `best_selling` is an explicit sort, so only the BM25 leg runs (team-search D8), in every search mode.
  Sort clause: `sold_count desc (missing 0)`, `created_at desc (missing last)`, `id asc`.
- **Filter.** `rating_avg >= min_rating` in every mode and in saved-search runs. `review_count = 0` documents have
  `rating_avg = 0` and are excluded by any floor above 0.
- **Facet.** `facets.ratings` has keys `"4"` and `"3"`, each counting documents matching the query and all filters except
  `min_rating` with `rating_avg` at or above the floor (cumulative); implemented as a global-filter aggregation (C7). Zero-count floors are still returned.
- **Pending values.** Every pending write sets `pending_at` (epoch millis, indexer clock); the listing upsert clears it. The sweep deletes documents with `pending_at` older than `TOMBSTONE_TTL` and no `status`; tombstones keep their own `tombstoned_at` purge.
- **Mapping.** New fields: `sold_count` (long), `rating_avg` (double), `review_count` (long), `pending_at` (date). The legacy `rating` float in the mapping stays but is unused; the filter reads `rating_avg`.

## Risks

- Reindex of existing indexes is additive (new fields), but listings without the fields count as `sold_count` 0 and
  `rating_avg` 0 until the owners' D2 backfill events arrive.
- Hot files shared with `search-attribution`: `internal/rpc/search.go` and `internal/index/opensearch.go`.

## Configuration

- `KAFKA_ORDER_TOPIC` default `order.events`; `KAFKA_ENGAGEMENT_TOPIC` default `engagement.events` (beside `KAFKA_LISTING_TOPIC`). The consumer group stays `KAFKA_CONSUMER_GROUP` (`team-search-indexer`).
- DLQ topics are the indexer's own `search-indexer.<topic>.dlq` (C17): `search-indexer.order.events.dlq`, `search-indexer.engagement.events.dlq` and `search-indexer.listing.events.dlq`, created by redpanda-init; the indexer never shares `order.events.dlq` with team-notification. The retry budget is the go-sdk consumer default `MaxAttempts` 5 with its backoff; there is no team-search env var for it.
- Envelope types: `platform.order.v1.ListingSalesChanged`, `platform.engagement.v1.ListingRatingChanged`.

## Clarifications

- C17 (human, 2026-11-08): DLQ topics are `search-indexer.<topic>.dlq`, created by redpanda-init (three topics); supersedes the earlier `<topic>.dlq` naming.

## Rollout order (C9)

Mapping changes are additive and applied by the indexer at startup. Order: (1) discovery-seo-contract; (2) deploy team-search with the new mapping and consumers, so the fields exist and the topics are subscribed; (3) deploy team-order and team-engagement with their migrations, whose backfill enqueues one event per listing. Backfill events that arrive before step 2 stay on the topic (the group reads from its committed offset), and events for unindexed listings become pending values. The group sets no reset offset in `kafkax.NewConsumerClient` (`platform-core/packages/go-sdk/pkg/kafkax/kafkax.go:132-138`) and franz-go defaults to `AtStart`, so a topic new to the existing group is read from its earliest offset. The 30 s NFR holds once the topic exists; a backfill older than the topic's retention is lost. A missing topic delays consumption and does not crash startup.

The tombstone `version` (the listing event's `occurred_at`) and the owners' `changed_at` come from different clocks; comparing them is accepted as is.
