## Why

Buyers cannot sort search results by what sells or filter them by star rating, although both numbers exist in the
platform (paid orders, verified reviews). Stories 1, 2 (search side) and 3 of initiative `discovery-seo` need team-search
to index `sold_count`, `rating_avg` and `review_count` and to serve the sort, filter and facet. The buyer UI is a separate
slice, `frontend-discovery-sort-rating` (team-frontend, depends on this one).

initiative: discovery-seo

## What changes

- **team-search indexer** consumes two more topics next to `listing.events`: `order.events` (`platform.order.v1.ListingSalesChanged`) and
  `engagement.events` (`platform.engagement.v1.ListingRatingChanged`), topics from `KAFKA_ORDER_TOPIC` (default `order.events`)
  and `KAFKA_ENGAGEMENT_TOPIC` (default `engagement.events`). Each event sets absolute values on the listing document by
  partial update (D1). A value for a listing with no document yet is stored as a pending value and merged on the first listing
  upsert (D2); pending documents are invisible and purged by the existing sweep after `TOMBSTONE_TTL` (C6). A per-field
  version from `changed_at` makes stale and redelivered events harmless.
- **Mapping** gains `sold_count`, `rating_avg`, `review_count` (additive, applied by the indexer at startup, C9).
- **`SearchListings` / `RunSavedSearch`**: `SORT_BY_BEST_SELLING`; `min_rating` accepted again (reverses D9 of team-search);
  `facets.ratings` floors 4 and 3 that ignore their own filter (C7); `SearchHit` carries the three fields.
- **Rewritten, not deleted**: team-search FEATURES entries `search.srm-a-minimum-rating-search-is-rejected` and
  `search.srm-the-ratings-facet-is-empty`, and e2e scenarios in `search/search_query_correctness.feature`.
- Spec deltas: new capability `search-bestseller-rating`; `search-query-correctness` (old rule removed, pointer); `search-read-model-deletes` (pending documents).
- Depends on `order-listing-sales` and `engagement-listing-rating-events` for the producers and, through them, on `discovery-seo-contract`.

## Capabilities

### New Capabilities
- `search-bestseller-rating`: indexing of sales and rating values, best-selling sort, rating filter and facet, hit fields.

### Modified Capabilities
- `search-query-correctness`: the requirement "Rating is not offered while no rating is indexed" is removed (rules moved).
- `search-read-model-deletes`: pending projection documents are invisible and purged after `TOMBSTONE_TTL`.

## Impact

- team-search: `internal/events/` (new handlers), `internal/app/indexer.go`, `internal/config/config.go`, `internal/index/opensearch.go`
  (mapping, `sortClause`, filter clauses, ratings aggregation, pending merge, purge), `internal/rpc/search.go` (drop the `min_rating`
  rejection, map hit fields), `FEATURES.yaml`. `internal/rpc/search.go` is shared with `search-attribution` (serialised).
- platform-e2e: `search/search_query_correctness.feature`, `search/search_read_model_deletes.feature`, new `search/search_best_selling_rating.feature`.
- Rollout order: contract, then team-search (mapping and consumers), then team-order and team-engagement with their backfill (design.md).

## Non-goals

- All buyer UI and URL parameters (slice `frontend-discovery-sort-rating`), a "Đánh giá cao" sort, learning-to-rank, floors other than 3 and 4 in the facet.
- Computing sales or ratings in team-search: values are owned and published absolute by team-order and team-engagement.
- Other filter or facet quality work (diacritics, zero-result rewrite, exact totals).

## Non-functional requirements

- `SearchListings` p95 with `sort=best_selling` or `min_rating` set regresses at most 10% against the current local-stack baseline for the same query, measured, non-gating.
- A `ListingSalesChanged` or `ListingRatingChanged` event is visible on the search document and in results within 30 s of being published.
- A poison event reaches its `search-indexer.<topic>.dlq` topic within 60 s on the e2e stack (5 attempts) and never blocks the partition.
- No `*_ENABLED` flag for the new consumers; the topics come from config with defaults `order.events` and `engagement.events`.
- The p95 of searches without the new sort or filter changes by at most 5% against baseline, measured, non-gating.

## Data classification

| Field | Purpose | Personal? | Consent basis | Retention |
|---|---|---|---|---|
| `sold_count` per listing | sort and display of popularity | No (aggregate per listing) | not applicable | lives with the search document; removed when the listing is deleted |
| `rating_avg`, `review_count` per listing | rating filter, facet | No (aggregate; no reviewer ids in the event) | not applicable | same as above |
| pending values for unindexed listing ids | merge on first upsert | No | not applicable | dropped when merged; pending docs are purged by the sweep once older than `TOMBSTONE_TTL` (default 336h) |

The events carry no buyer or reviewer ids and the consumer logs only `listing_id`, never contact data.

## Threat model

- Assets: ranking integrity (sold count and rating drive the sort and filter), search availability.
- Abuse cases:
  - A forged or replayed event inflates a listing's sold count or rating. Mitigation: topics are internal Kafka topics written only by the owners' outbox; absolute values with a `changed_at` guard make replays idempotent and stale values ignored.
  - A malformed event (empty id, negative count, rating outside 0..5, missing `changed_at`) corrupts documents. Mitigation: validated, then error, retry, DLQ; the document is untouched.
  - A flood of events for random ids fills the index with pending docs. Mitigation: pending docs are never returned by search (no `published` status), and are purged by the sweep after `TOMBSTONE_TTL`.
  - A buyer sends `min_rating` out of range to force an expensive or erroring query. Mitigation: validated to 0..5 (also for a saved search's stored value), `INVALID_ARGUMENT` otherwise.
- Authz rule: search is public read; no new authorisation. Draft-visibility rules of `SearchListings` are unchanged and apply to the new sort and filter. The consumers have no RPC surface.
