## Context

team-ai owns the tag taxonomy and classifier (in-memory registry, REST under `/api/v1/ai`; no gRPC, no gateway route). Listing events (`platform.listing.v1.ListingChanged`, `ListingBaseInfoChanged`) carry title, description, category and `Variant{id,name,sku,price,stock}` but no tags or attributes. team-search owns the OpenSearch read-model and its indexer. The search contract (`search.proto`) has `Facets{categories, price_ranges, ratings, sellers}` only.

## Decisions

**D1 Classify at index time, from team-search, over HTTP.** The indexer calls team-ai `POST /api/v1/ai/tags/classify-sku-hierarchy` (listing with variants) or `/tags/classify` (without) for each create/update, and `/tags/classify` for a base-info event. This is a service-to-service call (AGENTS.md rule 3: a call, never a shared DB) like the existing embed call to modelserve. The alternative, putting tags on the `Listing` proto and classifying in team-domain, would change the write-model contract and make team-domain depend on team-ai; rejected. `TAG_CLASSIFIER_URL` (empty = off) configures it; the compose wiring for the `team-search-indexer` service is `TAG_CLASSIFIER_URL=http://team-ai-svc:8000`.

**D2 Two shapes in the document.** `facet_tags`: flat `keyword` array of `group:slug` for SPU tags. `skus`: `nested`, one object per variant (`variant_id, sku_code, name, price, stock, is_in_stock, attrs[group:slug]`). One keyword field serves every facet group, so a newly promoted tag or group needs no mapping change, and a terms aggregation returns all groups at once. `nested` is what stops "navy AND 512GB" matching a listing whose navy variant is 256GB; the filter is one `nested` query holding every `sku.*` condition plus `is_in_stock = true`.

**D3 Filter contract without proto change: request filter keys.** `tag.<group>` and `sku.<group>` ride in the existing `SearchListingsRequest.filters` map (comma list = OR within a group). The handler validates group and slug patterns (`effectiveFilters`, shared by SaveSearch/RunSavedSearch), and the index turns them into clauses, never into raw field names.

**D4 Response needs an additive proto field.** Dynamic facets cannot be expressed in `Facets` today. Required additive change in `platform-core/packages/proto/platform/search/v1/search.proto` (non-breaking; fields 5 and 6):

```proto
// One facet group (e.g. "color") with its value buckets (key = tag slug).
message AttributeFacet {
  string group = 1;
  repeated FacetBucket buckets = 2;
}

message Facets {
  // ...existing fields 1-4 unchanged...
  repeated AttributeFacet tags = 5; // SPU tags by group; filter key "tag.<group>"; count = listings
  repeated AttributeFacet skus = 6; // in-stock variant attributes by group; filter key "sku.<group>"; count = listings
}
```

After it is merged and vendored, the only handler change is in `toFacets` (`internal/handler/search.go`): map `f.Tags` / `f.SKUs` (already computed by the index as `index.Facets.Tags/SKUs`) to `[]*searchv1.AttributeFacet`. The frontend reads `facets.tags` / `facets.skus` through a structural adapter in `lib/gateway/search.ts` that becomes a plain field read after regeneration. The gateway forwards the response untouched.

**D5 Facet counts.** `tags` count listings. `skus` runs a nested aggregation, restricted to in-stock variants that satisfy every active `sku.*` filter, then `reverse_nested` back to listings. So a selected `sku.capacity` narrows the color counts to variants of that capacity, and a group that is itself selected shows only its selected values (the UI offers the other values by deselecting). Labels are derived in the frontend (group title table, slug read as words) because the contract carries slugs only.

**D6 Failure handling.** A classifier error never fails ingestion (like the embedder): the document is upserted with `tags_pending = true` and the write script (`writeScript`) carries the stored `facet_tags` and `skus` forward, so an outage never wipes facets. Without a classifier configured (`TAG_CLASSIFIER_URL` empty) documents are written without tags.

**D7 Variant stock stays current.** `ListingStockChanged` carries `variants`; the indexer now applies each variant's stock to the matching nested `skus` entry (`is_in_stock = stock > 0`) inside the same `stock_version`-guarded script that sets the base stock (`UpdateStockWithVariants`). The earlier "variants are ignored" rule of the stock read-model (D13) no longer holds for the nested field; `hits[].stock` still reports the base stock only. Without it a sold-out variant would keep satisfying `sku.*` filters until the next `ListingChanged`.

## Mapping change and migration

The new fields are additive: `EnsureIndex` creates them on a fresh index and puts them (`additiveMapping`, same entries as `indexMapping`) onto an existing index at boot of both the server and the indexer, idempotently (the same additive-mapping mechanism the stock and created_at fields use, unchanged). No new index and no alias swap is needed, and `knn_vector`/`vector_pending` are untouched. `team-search-migrate` is golang-migrate for the saved-search Postgres database and has no part in OpenSearch.

Existing documents have no tags until re-indexed, so they simply do not match a facet filter and contribute no facet buckets. Backfill is a replay of `listing.events` (ADR-0005: the read-model is rebuildable): stop the indexer, reset the consumer group `team-search-indexer` to the earliest offset (`rpk group seek team-search-indexer --to start`), start it. The version guard makes the replay idempotent and safe against newer state; a base-info/pricing partial update never creates a document. A re-run after promoting new tags in team-ai refreshes the listings the same way.

## Findings that corrected the spec

- The first version of the promotion scenario said search "immediately" includes the promoted facet. Facets are stored per document at index time, so a promoted tag reaches only listings classified after the promotion (or re-indexed by a replay). The scenario now says that.
- team-ai's taxonomy registry is in memory (`TagClassifierService`): promotions are lost on restart and not shared between replicas. Persisting it is outside this change; until then a replay after a team-ai restart can drop a promoted tag from re-indexed listings.
- The e2e feature `ai/tag_classifier_taxonomy.feature` drives `TagClassifierService` in-process (imports `team-ai` code), not the running stack; it does not cover the search side, which the scenarios above add.

## Risks

- Classification adds one HTTP call per event to the indexer (budget `<10ms` server side, 2 s client timeout); a slow team-ai slows indexing, not queries.
- `facet_tags` terms aggregation asks for 200 buckets over all groups and returns at most 12 per group.
- The k-NN leg receives the same nested/terms filters inside its `filter`; a cluster that rejected it would make the engine fail open to lexical, and an integration test covers acceptance on OpenSearch 2.19.
