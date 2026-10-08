## Why

The retired `full_team_repo` had a correct search read-model; agora's `team-search` was rebuilt from an older base and
lost four parts of it (port audit `team-search.md` items 2, 3, 4 and 6). Verified in agora's current code:

- **Stock never reaches search.** `team-domain` writes a `ListingStockChanged` envelope to `listing.events` inside every
  reserve, release and TTL-sweep transaction (`openspec/specs/inventory-reservations`). `team-search`
  (`internal/consumer/listing.go`) declares `listingStockChangedType` but has no `case` for it, so the record falls to
  `default: return nil` and is dropped. The `listings` mapping has no `stock`, `toDoc` ignores `Listing.stock`, and
  `SearchHit` carries only `listing_id` and `score`. A `filters["in_stock"]` today becomes a `term` on a field that
  does not exist and matches nothing.
- **A deleted listing can come back.** `OpenSearchIndex.Delete` is a hard delete with no version tombstone. Once
  OpenSearch forgets the delete (`index.gc_deletes`, 60 s by default), a late or replayed `ListingChanged` is accepted
  as new and the listing is searchable again. The three delete paths (`ListingChanged` DELETED,
  `ListingBaseInfoChanged` DELETED, `ListingStatusChanged` REJECTED) all use it.
- **"Newest" is not newest.** `SORT_BY_NEWEST` sorts by `_id desc` in both `Search` and `SearchVector`; listing ids
  are UUIDs, and the index holds no creation time. With a query, the default hybrid mode also re-orders the sorted legs
  by RRF, so no explicit sort survives fusion.
- **Rating is a dead control.** No rating is indexed anywhere: `Listing` has no rating field, `team-search` consumes
  only `listing.events`, and `toDoc` never sets `Rating` (every full upsert writes `rating: 0`). Yet `min_rating` is
  passed into both retrieval legs (so `min_rating > 0` always matches nothing) and the `ratings` facet always returns
  four buckets of count 0, which the frontend renders as a filter.

Every buyer search, sort and saved-search run goes through this read-model.

## What Changes

- **platform-core (contract, lands first, alone):** add `optional int32 stock = 3;` to `platform.search.v1.SearchHit`
  (field 3 is unused in agora's `search.proto`; `optional` so "unknown" is distinguishable from 0). Additive;
  `buf lint` and `buf breaking` must pass. No request field: `in_stock` travels in the existing `filters` map, which
  is also what saved searches persist. `SearchListingsRequest` field 10 stays free.
- **Re-vendor:** `team-search` and `team-gateway` (required: the gateway re-serialises Connect JSON from its generated
  types, so an unknown field would be dropped at the edge), plus every other repo that vendors
  `platform/search/v1` so the copies do not drift. No behaviour change outside `team-search`.
- **team-search, stock projection:** index `stock` and `stock_version`; apply `ListingStockChanged` as a guarded
  partial update (never creates a document, never touches a deleted one, an older event is a no-op, a malformed one
  goes through the existing retry-then-DLQ path); a `ListingChanged` takes `Listing.stock` only when it is newer than
  the stored stock, otherwise carries the stored stock forward; `filters["in_stock"]="true"` restricts every retrieval
  leg, the totals and the facets to `stock > 0`; `SearchHit.stock` filled on `SearchListings` (every search mode) and
  `RunSavedSearch`.
- **team-search, delete tombstones:** every delete path writes a version-guarded tombstone document instead of
  deleting; an event at or before the delete cannot resurrect the listing, partial and stock updates never revive
  it, and no read path returns it. The indexer purges tombstones older than `TOMBSTONE_TTL` (default 14 days) every
  `TOMBSTONE_PURGE_INTERVAL`.
- **team-search, newest:** index `created_at` from the `CREATED` event's `occurred_at`, kept across later upserts;
  `SORT_BY_NEWEST` orders by `created_at` desc with an `id` tie-break, and a key sort is honoured in every search mode
  (hybrid serves key-sorted requests from the lexical leg instead of fusing).
- **team-search, honest rating:** `min_rating` other than 0 is rejected with `INVALID_ARGUMENT` before any retrieval
  leg runs; the `ratings` facet is returned empty; full upserts stop writing `rating`. The mapping field stays.
- **team-search, mapping evolution:** the new fields are added to an existing index by an idempotent put-mapping at
  startup and are part of the create mapping; replaying `listing.events` backfills them.
- **team-frontend, rating control removed:** the search page no longer renders the rating facet group, chip or
  drawer option, never sends `minRating`, and ignores an old `?rating=` URL parameter (the page renders normally).
  Without this, the `INVALID_ARGUMENT` above would turn every old rating link into an error page.
- **platform-e2e + `team-search/FEATURES.yaml` + `team-frontend/FEATURES.yaml`:** one e2e scenario per spec scenario, driven by real checkout / cancel /
  update / delete flows through the gateway and, for out-of-order cases, by publishing envelopes to `listing.events`.
- **BREAKING (behavioural, public API):** `SearchListings` with `min_rating > 0` now fails with `INVALID_ARGUMENT`
  instead of returning nothing; the `ratings` facet is empty; `filters["in_stock"]` with a value other than `"true"`
  is `INVALID_ARGUMENT`. The frontend change above ships in the same change so no buyer-facing URL breaks.

## Capabilities

### New Capabilities

- `search-stock-read-model`: the read-model holds each listing's current stock from `ListingStockChanged` and
  `ListingChanged`, never regresses under reordering or redelivery, returns it per hit and filters on it.
- `search-read-model-deletes`: a deleted listing stays deleted in the read-model (version-guarded tombstones, sticky
  against partial and stock updates, invisible to every read path) until a configurable retention purges it.
- `search-query-correctness`: "newest" means most recently created in every search mode, and the rating filter and
  facet are honest while no rating is indexed.
- `frontend-search-rating`: the buyer search page offers no rating filter and opens an old `?rating=` link as a normal
  results page (FEATURES owner `team-frontend`).

### Modified Capabilities

_None._ `openspec/specs/listing-read-visibility` (published-only default, owner-only drafts, saved-search identity)
already holds in agora's code and is unchanged; the new requirements build on it.

## Impact

- **team-search:** `internal/index/opensearch.go` (mapping, put-mapping, scripted upsert, stock update, tombstone,
  purge, filter builder, sort, facets), `internal/consumer/listing.go` (stock case, delete paths, created time),
  `internal/retrieval/engine.go` + `fusion.go` (stock through fusion, key sort bypass), `internal/handler/search.go`,
  `saved_search.go`, `visibility.go` (`in_stock` and `min_rating` validation, `SearchHit.stock`),
  `internal/config` (+ `.env.example` drift gate), `cmd/indexer` (purge loop), `README.md`, `FEATURES.yaml`.
- **platform-core:** `packages/proto/platform/search/v1/search.proto` only.
- **team-frontend:** `src/features/search/url.ts` (drop `rating` from parsed state, hrefs and the active-filter
  count), `data.ts` (no `minRating`), `FilterSidebar.tsx` (no ratings group or hidden field, inline and drawer),
  `ActiveFilters.tsx`, `SearchBlocks.tsx`, `SortBar.tsx`, `src/lib/gateway/search.ts` (no `minRating` option), their
  tests, `FEATURES.yaml`.
- **team-gateway** and the other repos vendoring `platform/search/v1`: re-vendor and regenerate only.
- **platform-e2e:** new features, steps and an envelope publisher; a compose overlay with short tombstone retention.
- **Data:** the `listings` index gains four fields by put-mapping (no reindex). Existing documents have no stock and no
  creation time until a replay of `listing.events`; existing hard deletes have no tombstone (they predate the guard).
- **Architecture rules:** Rule 3 (team-search reads only its own index and the event stream), Rule 4 (one additive
  field in platform-core, re-vendored, never hand-edited), Rule 5 (state-change events on Kafka, unchanged), Rule 2
  (the gateway forwards the new field unchanged).

## Non-goals

- `listing_ids` restriction, `request_id` and `model_version` on search responses (audit item 1, AI-first serving
  attribution; separate change).
- Visibility, published-only default and saved-search hardening (audit items 5 and 7; already in agora, see
  `openspec/specs/listing-read-visibility`).
- Per-variant stock in the index (decided: base-listing stock only, design D13). The read-model mirrors the base
  `stock` the domain publishes (`Listing.stock`, `ListingStockChanged.stock`); variant entries in the event are
  ignored. No agora listing flow or seed uses variants today.
- Ranking or sorting by stock; a rating source (review events or calls to `team-engagement`).
- Any change to `team-domain`'s publisher, to gateway routing or scopes, or to the frontend beyond removing the rating
  control (no stock display or in-stock toggle in the UI).
- A dedicated backfill job; replaying `listing.events` is the rebuild path.
