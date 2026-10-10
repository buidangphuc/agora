> **Order.** Group 1 (contract) runs first, serially and alone; nothing else starts until 1.1 is merged in
> `platform-core`. Then group 2 (re-vendor) and, in parallel, three tracks: **CODE team-search** (group 3; 3.1 → 3.2 →
> 3.3 are sequential because they share `internal/index/opensearch.go`, 3.4–3.9 follow), **CODE team-frontend**
> (group 4, independent of the proto change and of group 3; deploy it before or with group 3) and **E2E** (group 5,
> `platform-e2e` + `team-search/FEATURES.yaml` + `team-frontend/FEATURES.yaml`, written from the specs, red first).
> Group 6 is the convergence gate. One fix = one commit; each repo's CI-equivalent checks run before every commit
> (`platform-core`: `make check`; `team-search` / `team-gateway`: `make check`; `team-frontend`: `npm run check`;
> `platform-e2e`: `make features-check` + lint).

## 1. Contract (platform-core) — serial, lands first

- [x] 1.1 Add `optional int32 stock = 3;` to `SearchHit` in `platform-core/packages/proto/platform/search/v1/search.proto` with a comment (listing's base stock in the read-model; absent = unknown; filled on `SearchListings` and `RunSavedSearch`), and a comment on `SearchListingsRequest.filters` documenting the `in_stock="true"` key; no field renumbered or removed; verify `make -C platform-core check` (lint + breaking) passes and `git diff --stat` touches only `search.proto`

## 2. Re-vendor (after 1.1)

- [x] 2.1 Re-vendor `proto/` and run `buf generate` in `team-search` and `team-gateway`; verify `go build ./...` and `make check` pass in both and the diff shows only vendored proto and generated code, no hand edits
- [x] 2.2 Re-vendor `platform/search/v1` in the other repos that vendor it (`team-ai`, `team-analytics`, `team-chat`, `team-domain`, `team-engagement`, `team-frontend`, `team-identity`, `team-notification`, `team-order`, `team-payment`) and regenerate where generation is committed; verify each repo's build/check (`make check` for Go repos, `npm run check` for `team-frontend`, `make check` or `pytest` for `team-ai`) and that the `repo-doctor` proto-drift check reports no stale `search.proto`
- [x] 2.3 Add a `team-gateway` test that a `SearchListings` response with `hits[].stock = 0` and one with no stock round-trip through the edge as `"stock": 0` and as an absent key in Connect JSON; verify `make check` in `team-gateway`

## 3. team-search (code track)

- [x] 3.1 Mapping evolution (D10): add `stock` (integer), `stock_version` (long), `created_at` (date, epoch millis), `tombstoned_at` (date) to `indexMapping`, and an idempotent `PUT _mapping` for those four fields in `EnsureIndex` when the index exists; verify an index integration test (throwaway OpenSearch) that an index created with the old mapping gains the fields and a second `EnsureIndex` is a no-op, and that two concurrent `EnsureIndex` calls both succeed
- [x] 3.2 One write guard (D1, D5): replace the external-version `Upsert` with a `scripted_upsert` guarded on `_source.version`, add the tombstone rules to it and to `versionGuardScript`, and make `Delete(ctx, id, version)` write the tombstone document through the same script; switch the three delete paths in `internal/consumer/listing.go` to it; stop writing `rating`; verify index tests: stale/equal/unversioned upsert after a tombstone is a no-op, newer upsert replaces it, partial update on a tombstone is a no-op at any version, delete over a newer tombstone keeps the newer version, a plain stale upsert on a live doc is still a no-op
- [x] 3.3 Stock projection (D2): add `Index.UpdateStock(ctx, id, stock, version)` (scripted update without upsert, `stock_version` guard, tombstone no-op, 404 acknowledged); add the `ListingStockChanged` case to the consumer with validation (empty id, negative stock, missing `occurred_at` → error); make the upsert script take `Listing.stock` only when newer than `stock_version`; verify consumer + index tests: in-order and reversed stock events end at the newest value, a stock event for a missing doc creates nothing, an older `ListingChanged` keeps the newer stock while applying its title, a newer one replaces it, malformed events return an error (and, with the existing consumer tests, reach the DLQ fake)
- [x] 3.4 `created_at` (D7): pass the CREATED envelope's `occurred_at` into the upsert, set-if-absent-or-earlier also for a stale CREATED, never on a tombstone; change `SORT_BY_NEWEST` in `Search` and `SearchVector` to `created_at desc (missing _last), id asc`; verify tests: UPDATED does not change `created_at`, a CREATED delivered after a newer UPDATED still records it, the sort body of both legs, and an index integration test ordering three docs (one without `created_at`)
- [x] 3.5 `in_stock` (D4): consume the `in_stock` key in `buildFilterClauses` as `range stock > 0` (never a raw term), and validate it in `effectiveFilters` (`"true"` only, else `INVALID_ARGUMENT`); verify unit tests on the clause list (both legs, including the k-NN `filter`), handler tests for `SearchListings`, `SaveSearch` and `RunSavedSearch` with valid and invalid values, and an index integration test that a doc without `stock` does not match
- [x] 3.6 Stock on hits (D3): decode `stock` into `index.Hit` (pointer), carry it through `retrieval.Candidate`, `RRF`, rerank and pagination, and map it to `SearchHit.stock` in `SearchListings` and `RunSavedSearch`; verify engine tests that fused, reranked, lexical-fallback and semantic-fallback hits keep their stock, and handler tests for present / zero / absent stock
- [x] 3.7 Key sort bypasses fusion (D8): in `Engine.Execute`, a hybrid (explicit or defaulted) request with `sort_by` `NEWEST`, `PRICE_ASC` or `PRICE_DESC` is served by the lexical leg with that sort and page; verify engine tests that such requests never call `RRF` and that `RELEVANCE`/unspecified still fuse
- [x] 3.8 Rating honesty (D9): reject `min_rating != 0` with `INVALID_ARGUMENT` in `SearchListings` before the engine runs, drop the `ratings` aggregation and return an empty `Ratings` slice; verify handler tests in every search mode (index never called), facet tests that `ratings` is an empty non-nil list on `SearchListings` and `RunSavedSearch`
- [x] 3.9 Tombstone purge (D6): add `TOMBSTONE_TTL` (default `336h`) and `TOMBSTONE_PURGE_INTERVAL` (default `1h`) to `internal/config` with fallback-plus-WARN, `Index.PurgeTombstones(ctx, olderThan)` (`delete_by_query` on `status=deleted AND tombstoned_at < cutoff`), and a purge loop in `cmd/indexer` that logs the effective values and each purge count; update `.env.example` and README (runbook: replay after deploy, retention trade-off); verify config tests (default, override, invalid → default + warning), `make check-env`, an index integration test that only expired tombstones are removed, and a loop test with a fake clock
- [x] 3.10 Run `make -C team-search check` (env drift, gofmt, vet, tests) and the index integration tests against a throwaway OpenSearch; verify all green and record the output in the commit message

## 4. team-frontend (code track)

- [x] 4.1 Remove rating from the search URL model (D12): drop `rating` from `SearchState`, `parseSearchParams` (an incoming `rating` is ignored), `buildSearchHref`, `activeFilterCount` and `clearFiltersHref` in `src/features/search/url.ts`, and the `rating` defaults in `SortBar.tsx`; verify `url.test.ts` cases: `?q=x&rating=4` parses equal to `?q=x`, no built href contains `rating=`, the active-filter count ignores it
- [x] 4.2 Stop sending a minimum rating: remove `minRating` from `data.ts` and the `minRating` option from `searchListings` in `src/lib/gateway/search.ts`; verify `search.test.ts` asserts the request has `minRating` 0 for every call, including one built from a `?rating=4` state
- [x] 4.3 Remove the rating UI: the ratings group (inline card and drawer) and the `rating` hidden field in `FilterSidebar.tsx`, the rating chip in `ActiveFilters.tsx`, `currentRating` in `SearchBlocks.tsx`, and `ratings` from the `hasAnyFacet` check; verify `FilterSidebar.test.tsx` / `SearchBlocks.test.tsx` render no `facet-ratings` group and no "Đánh giá" text even when `facets.ratings` is non-empty
- [x] 4.4 Run `npm run check` (biome, `tsc --noEmit`, token check, vitest) in `team-frontend`; verify green and record the output in the commit message

## 5. E2E track (platform-e2e + team-search/FEATURES.yaml + team-frontend/FEATURES.yaml)

- [x] 5.1 Add an envelope publisher to `platform-e2e/tests/e2e/flows` (protobuf field encoder for `EventEnvelope`, `ListingChanged`, `ListingStockChanged`, `ListingStatusChanged` with a chosen `occurred_at`; publish keyed by listing id; re-publish raw record bytes), a helper that waits until the `team-search-indexer` group's committed offset passes a given record, a `listing.events.dlq` reader, and an OpenSearch `_doc` reader; verify unit tests that encoded envelopes decode with the existing `_fields` decoder
- [x] 5.2 Add a compose overlay `platform-e2e/compose/search-tombstones.override.yaml` setting `TOMBSTONE_TTL=30s` and `TOMBSTONE_PURGE_INTERVAL=2s` on `team-search-indexer`, wired the same way as `order-inventory.override.yaml`; verify the indexer logs the effective values at start on the e2e stack
- [x] 5.3 Add `team-search/FEATURES.yaml` entries for the three capabilities with one `acceptance` line per spec scenario (names 1:1) and `covered_by` pointing at the new features; verify `make -C platform-e2e features-check` validates the manifest
- [x] 5.4 Add `tests/e2e/features/search/search_stock_read_model.feature` + steps for the 12 `search-stock-read-model` scenarios (real checkout / cancel / `UpdateListing` through the gateway; black-box events via 5.1; bounded polling 30 s, DLQ 60 s); verify the scenarios run red before 3.3–3.6 land and green after
- [x] 5.5 Add `tests/e2e/features/search/search_read_model_deletes.feature` + steps for the 7 `search-read-model-deletes` scenarios (real `DeleteListing`; stale, redelivered and newer partial events via 5.1; purge observed in OpenSearch under the 5.2 overlay) in the serial lane; verify red before 3.2/3.9 and green after
- [x] 5.6 Add `tests/e2e/features/search/search_query_correctness.feature` + steps for the 5 `search-query-correctness` scenarios (real create / update / publish for newest, black-box late CREATED, `min_rating` and facet checks through the gateway and `RunSavedSearch`); verify red before 3.4/3.7/3.8 and green after
- [x] 5.7 Add `team-frontend/FEATURES.yaml` entries for `frontend-search-rating` (one `acceptance` line per scenario, names 1:1) and `tests/e2e/features/frontend/search_rating_removed.feature` + steps + page-object additions (filter sidebar groups, active chips, sort/pagination link hrefs) for its 2 scenarios through :3000; verify `make -C platform-e2e features-check` and the scenarios run red before group 4 and green after
- [x] 5.8 Re-run the existing search features (`buyer/search_price_sort.feature`, `frontend/search_facets.feature`, `search/hybrid_retrieval.feature`, the visibility and saved-search features) and the inventory feature against the new code; verify they stay green, and fix any step that relied on the four `ratings` buckets, the `facet-ratings` group (also listed in `team-search/FEATURES.yaml` `search.facets` key elements) or on `_id` ordering

## 6. Convergence gate

- [x] 6.1 Run the full e2e suite twice with `-n 4` (serial lane for 5.5) against the real stack; verify green both times and that every flake is root-caused
- [x] 6.2 Run `make -C platform-e2e spec-check CHANGE=port-search-read-model-correctness` and `openspec validate port-search-read-model-correctness --strict`; verify both pass and all 26 scenarios are `status: automated`
- [x] 6.3 Run the `contract-boundary-reviewer` over the `team-search`, `team-gateway`, `team-frontend` and `platform-core` diffs; verify no blocking finding (no business logic in the gateway or frontend, proto change additive and alone)
- [x] 6.4 With human approval, retire the superseded carried-over changes `openspec/changes/search-stock-events` and `openspec/changes/search-correctness-and-privacy` (their remaining scope is either in this change or already in `listing-read-visibility`); verify `openspec list` no longer shows them

## Evidence (2026-10-08)

- **Contracts, each landed first and alone:**
  - 86d8367 (SearchHit.stock) and 9dc0e5a (OrderCancelled); both pass `make lint-proto` and `make breaking`.
  - Re-vendored with 4138f8d (search.proto, all 12 copies identical) and 5924f6c (order.proto into team-order and team-payment).
- **Local deploy:**
  - The 0006 sign pre-check on payment_db returned 0 violating rows (849 ledger rows); 0006 applied.
  - Topic order.events.payment-settlement.dlq created.
  - The settlement consumer started at the latest offset.
- **e2e:** payment 32/32 and search 26/26 scenarios automated (spec_sync --strict).
  - Final gate with the order-inventory, payment-ledger and search-tombstones overlays: 363 passed twice (`-n 4`), then 27 passed in the destructive lane.
- **Defects the live runs found, fixed:**
  - A local override shadowed the committed topic list, so the DLQ topic was missing and the consumer stalled on a poison record, as designed.
  - Fixes: e8b0f8b (analytics.events created by redpanda-init) and the override dropped.
  - Two step-binding bugs in the payment e2e steps (fixed after the first live run).
  - gitops never enabled Kafka for team-payment, so in a cluster no seller would be credited. Fixed in 5040cd4 (DLQ topic in the redpanda job too).
- **Flakes, each root-caused:**
  - The facet sidebar was read before it streamed in (de68eab).
  - A listing-B candidate loop deleted its last candidate (d955004).
  - The collection form was typed into before hydration (c3097b1).
  - The kind GitOps cluster had silently auto-started after a Docker restart (4.3 GB of the 11.7 GB VM), which caused 1–7 s Postgres commits. It is stopped and its restart policy cleared.
- **Reviews, no blocking findings:**
  - Boundary follow-up applied (gitops Kafka and DLQ).
  - Auth follow-ups recorded as open: an `order.events` produce ACL, the payment-id existence oracle on RefundPayment, and a seller partial refund closing the payment (the spec allows it; product decision pending).
