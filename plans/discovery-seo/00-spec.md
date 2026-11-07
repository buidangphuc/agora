# 00-spec — Discovery and SEO: best-selling sort, rating filter, search attribution, cart cross-sell, SEO

initiative: discovery-seo

Source: backlog Q19, Q26, Q32 and Q34 (`plans/pm-review-2026-10/BACKLOG.md` on branch `docs/pm-review-backlog`). Evidence: `growth-retention.md` #6, `ai-data.md` #3 and #9, and `buyer-journey.md` #7.
Base: `origin/main` at c5835247.
Q25 (personalised home row) is left out because it depends on the consent decision in Q33.

## Goal
- **Search sort and filter.** A buyer can sort search results by "Bán chạy" and filter them by star rating. Both use real numbers projected from orders and verified reviews.
- **Attribution.** Every search-result impression and click carries the server's `request_id` and `model_version`. This lets analytics join a click back to the search that produced it and compare ranking versions, as recommendations already can.
- **Cart cross-sell.** The cart shows a "Thường được mua cùng" row that is driven by the cart's contents. Every key in the recommendation placement config either does something or is deleted.
- **SEO.** Public pages (home, PDP, shop, category/search) get real titles, descriptions, canonical URLs and Open Graph tags under the brand "Agora". Private pages are noindex, and the site serves `robots.txt` and `sitemap.xml`.

## User stories

### 1. Sort by best-selling
- **Given** listings with different paid quantities.
- **When** the buyer picks "Bán chạy" on `/search`.
- **Then** the URL carries `sort=best_selling`. Results are ordered by `sold_count` descending, with ties broken by `created_at` descending and then `id`.
- **Edge cases:**
  - Listings with no sales have `sold_count = 0` and sort last.
  - Explicit sorts already use only the BM25 leg (D8 of team-search), and this sort does too.

### 2. Sold count reflects paid minus cancelled or refunded
- **Given** order O for 2 units of listing L.
- **When** O is paid, then cancelled from PAID, or a return of O is refunded.
- **Then** L's `sold_count` goes +2 on payment and −2 on the cancel or refund.
- **Edge cases:**
  - The count never goes below 0.
  - A cancel from a status before PAID changes nothing.
  - A redelivered event changes nothing, because the projection lives in team-order and is keyed by order.
  - The search document shows the new value within 30 s.

### 3. Filter by rating
- **Given** listings with verified, non-hidden reviews.
- **When** the buyer picks "Từ 4 sao" or "Từ 3 sao" in the filter sidebar.
- **Then** the URL carries `minRating=4` or `minRating=3`, and only listings whose `rating_avg` is at least that value are returned.
- **Edge cases:**
  - Listings with no reviews are excluded by any rating filter.
  - The ratings facet shows counts per floor.
  - An old `rating=` link still renders.
  - The value is the same one `GetListingRatingSummary` returns (Q2 rule: verified, not hidden).

### 4. Search results are attributable
- **Given** a search on `/search`.
- **When** results render and the buyer clicks one.
- **Then** the impression and click beacons carry `impressionId = SearchListingsResponse.request_id` and `modelVersion = SearchListingsResponse.model_version`.
- **Then** paging or re-sorting gets a new `request_id`.
- **Then** team-analytics answers "impressions, clicks, CTR for `search_results` per `model_version`".
- **Edge case:** when the hybrid or semantic leg falls back to lexical, `model_version` names the mode that actually ran, not the requested one.

### 5. Cart cross-sell
- **Given** a cart with items.
- **When** the buyer opens `/cart`.
- **Then** a "Thường được mua cùng" row shows up to 8 listings from the `cart_cross_sell` placement, using every cart line as context.
- **Then** the row excludes listings already in the cart, out-of-stock listings, and more than one listing per seller.
- **Then** its beacons carry the recommendation `request_id` and `model_version`.
- **Edge cases:**
  - With an empty cart or an empty result, the row is hidden.
  - If recommendations fail, the cart still renders.

### 6. Placement strategies and filters are real
- **Given** `placements.yaml`.
- **When** a placement lists `category_popular`, the strategy returns popular listings from the seed's category (or the cart items' categories). `cart_cross_similarity` returns items similar to the cart lines.
- **Then** the `filters` keys `in_stock_only`, `deduplicate_sellers`, `exclude_seed` and `exclude_cart_items` change the output when toggled.
- **Then** keys the loader still ignores are deleted from the file and from the docs.

### 7. SEO metadata
- **Given** a crawler or a chat app unfurling a link.
- **When** it fetches `/listing/<id>`, `/shop/<id>`, `/search?category=<id>` or `/`.
- **Then** the page has:
  - `<title>` set to `<page> | Agora`;
  - a Vietnamese meta description;
  - `link rel=canonical` as an absolute URL from `NEXT_PUBLIC_SITE_URL`;
  - `og:title`, `og:description`, `og:url`, `og:site_name=Agora` and `og:image` (the listing's first image or the shop logo);
  - `og:type=product` on the PDP.
- **Edge cases:**
  - A missing listing returns 404 with no OG tags.
  - A free-text search (`q=`) is `noindex,follow`.
  - Category pages are indexable.

### 8. Private pages and crawl files
- **Given** a crawler.
- **When** it fetches `/robots.txt`, `/sitemap.xml`, or any page under `account`, `cart`, `checkout`, `seller`, `admin`, `login`, `register`, `chat`, `notifications`, `favorites` or `dev`.
- **Then** `robots.txt` disallows those prefixes and points to the sitemap.
- **Then** those pages carry `robots: noindex,nofollow`.
- **Then** `sitemap.xml` lists home, every category, and the active listings and their shops, up to 5,000 listings, newest first.
- **Then** no public UI string says "Marketplace Showcase", "Polyrepo" or "BATDONGSAN SINH VIÊN". The site has a favicon.

## Non-goals
- Learning-to-rank, or search rows in `rank_training` (LTR stays Later in Q26).
- Diacritic-insensitive matching, zero-result rewrite, exact `Total`, and facet quality (the rest of buyer#7).
- A "Đánh giá cao" sort.
- Live stock eligibility through team-search (C3-U1). Stock filtering uses the candidate payload's `in_stock`, as it does today.
- Q25 personalised home row; Q33 consent reach.
- Structured data (JSON-LD), hreflang, and AMP.

## Architecture decisions
- **D1: The owner publishes absolute values; search projects them.**
  - team-order keeps a `listing_sales` projection (paid quantity minus quantity cancelled from PAID or refunded) and emits `ListingSalesChanged{listing_id, sold_count}` on `order.events` through its outbox.
  - team-engagement emits `ListingRatingChanged{listing_id, average_rating, review_count}` on `engagement.events` whenever a verified review is created or hidden.
  - The team-search indexer consumes both topics and sets the fields on the document by partial update.
  - Why: absolute values make redelivery harmless, and team-search has no database for a ledger. Each owner keeps its own rule (Q2 verified rule, Q10 cancel and refund).
- **D2: Backfill.**
  - Each owner's migration seeds its projection from existing rows and enqueues one event per listing in the outbox.
  - The search indexer accepts a value for a listing whose document does not exist yet: it stores the value in a pending doc and merges it on the next listing upsert.
  - Why: existing listings would otherwise show 0 or no rating until they get their next sale or review.
- **D3: Rating filter returns.**
  - This reverses team-search D9: the handler no longer rejects `min_rating`.
  - The mapping gains `rating_avg` (float), `review_count` (integer) and `sold_count` (integer).
  - The ratings facet is filled from `rating_avg` floors 3 and 4.
  - The FEATURES entries that assert the rejection (`search.srm-*`, `discovery.search-rating-*`) are rewritten, not deleted.
- **D4: Search attribution.**
  - team-search mints `request_id` (UUID v4 hex) per `SearchListings` / `RunSavedSearch` call.
  - `model_version` is `<mode>[+rerank]@<SEARCH_RANKER_VERSION>`. `<mode>` is the mode that actually ran (`lexical|semantic|hybrid`). `SEARCH_RANKER_VERSION` is an env var, default `v1`.
  - The fallback flag that `engine.Execute` already returns is used for this, instead of being discarded.
  - team-ai's own `SearchListings` servicer fills the same two fields.
  - The frontend passes both to `TrackImpression`/`TrackLink` exactly as `RecommendationsRow` does.
  - team-analytics adds a `SearchPerformance` query that mirrors `RecommendationPerformance` (join on `impression_id` + `listing_id`, placement `search_results`).
- **D5: Cart context in recommendations.**
  - `RecommendRequest` gains `repeated string context_listing_ids`, which carries the cart lines.
  - `seed_listing_id` stays for single-anchor contexts.
  - The gateway caps the list at 50 ids.
- **D6: Placement loader honours `filters` and deletes the rest.**
  - `in_stock_only`, `deduplicate_sellers`, `exclude_seed` and `exclude_cart_items` are implemented in `ranking/filter.py`.
  - `name`, `enabled`, `timeout_ms`, `allow_cache` and `tier` are removed from the file and from the docstring, unless the loader already uses them. Per the stale-config rule they are deleted, not fixed.
- **D7: SEO lives in Next.js metadata only.** The root layout gets:
  - `metadataBase` from `NEXT_PUBLIC_SITE_URL` (default `http://localhost:3000`);
  - title template `%s | Agora`;
  - a default title and description.

  Public pages use `generateMetadata` and reuse each page's existing cached loader (`loadListing`, `getStorefront`, `loadCategories`), so there is no extra gateway round-trip. `app/robots.ts` and `app/sitemap.ts` are added; the sitemap reads listings through the existing search gateway client, sorted by `NEWEST`. Private route-group layouts export `robots: {index:false, follow:false}`. The existing `referrer: "no-referrer"` metadata stays, and `referrerMetadata.test.ts` must stay green.

## Contracts
```proto
// platform/search/v1/search.proto
enum SortBy { ... SORT_BY_BEST_SELLING = 5; }      // sold_count desc, created_at desc, id
message SearchListingsResponse { ... string request_id = 4; string model_version = 5; }
message RunSavedSearchResponse { ... string request_id = 4; string model_version = 5; }
message SearchHit { ... int64 sold_count = <next>; double rating_avg = <next>; int64 review_count = <next>; string seller_id = <next>; }

// platform/order/v1/order.proto — order.events
message ListingSalesChanged {
  string listing_id = 1;
  int64 sold_count = 2;                          // absolute, >= 0
  google.protobuf.Timestamp changed_at = 3;
}

// platform/engagement/v1/events.proto — engagement.events
message ListingRatingChanged {
  string listing_id = 1;
  double average_rating = 2;                     // verified, non-hidden reviews; 0 when review_count = 0
  int64 review_count = 3;
  google.protobuf.Timestamp changed_at = 4;
}

// platform/recommendation/v1/recommendation.proto
message RecommendRequest { ... repeated string context_listing_ids = 6; }  // cart lines; >50 ids → INVALID_ARGUMENT

// platform/analytics/v1/analytics.proto — admin-only, mirrors GetRecommendationPerformance
rpc GetSearchPerformance(GetSearchPerformanceRequest) returns (GetSearchPerformanceResponse);
// request: window_hours (0 → 24, max 168); response: window_hours, attribution_window_hours, repeated RecommendationPerformanceRow rows (placement_id = search_results)
```
The field numbers in `SearchHit` and `SearchListingsResponse` are the next free ones, chosen by the contract author.
Existing contracts consumed unchanged:
- `order.events/OrderPaidEvent`, `OrderCancelled`, `ReturnRefunded`;
- `engagement.events/ReviewCreated`, `ReviewHidden`;
- `EngagementService.GetListingRatingSummary`;
- `RecommendationService.Recommend`;
- `SharingService.ResolveShareLink`;
- `analytics.events/TrackingEvent` (fields 11 `impression_id` and 12 `model_version` already exist).

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contract | discovery-seo-contract | platform-core | – | provides: order.events/ListingSalesChanged, engagement.events/ListingRatingChanged, SortBy.BEST_SELLING, search request_id/model_version, RecommendRequest.context_listing_ids | platform-core (+ regenerated stubs in consumers) |
| sold count projection | order-listing-sales | team-order | discovery-seo-contract | provides: order.events/ListingSalesChanged; uses: order.events/OrderPaidEvent, OrderCancelled, ReturnRefunded | team-order |
| rating summary events | engagement-listing-rating-events | team-engagement | discovery-seo-contract | provides: engagement.events/ListingRatingChanged | team-engagement |
| best-selling sort + rating filter (backend) | search-bestseller-rating | team-search | order-listing-sales, engagement-listing-rating-events | uses: order.events/ListingSalesChanged, engagement.events/ListingRatingChanged | team-search |
| "Bán chạy" + rating filter UI | frontend-discovery-sort-rating | team-frontend | search-bestseller-rating | uses: SearchService.SearchListings | team-frontend |
| search attribution | search-attribution | team-search | discovery-seo-contract | uses: SearchService.SearchListings | team-search, team-ai, team-frontend, team-analytics |
| listing status in the analytics export | analytics-listing-status-export | team-analytics | – | uses: listing.events/ListingChanged | team-analytics |
| candidate attributes in the recsys read path | recsys-candidate-attributes | platform-recsys | analytics-listing-status-export | – | platform-featurestore, platform-recsys |
| cart cross-sell + real placement config | recsys-cart-cross-sell | team-ai | discovery-seo-contract, recsys-candidate-attributes | uses: RecommendationService.Recommend | team-ai, team-gateway, team-frontend |
| SEO metadata, robots, sitemap, brand | frontend-seo | team-frontend | discovery-seo-contract | uses: SearchService.SearchListings (SearchHit.seller_id) | team-frontend, team-frontend-seller, team-frontend-admin |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-search/internal/rpc/search.go`: search-bestseller-rating and search-attribution.
- `team-frontend/src/features/search/SearchBlocks.tsx` and `team-frontend/src/app/(shop)/search/page.tsx`: search-bestseller-rating, search-attribution and frontend-seo.
- `team-frontend/FEATURES.yaml`: every slice that touches the frontend.
- `team-search/FEATURES.yaml`: search-bestseller-rating and search-attribution.

## Out of initiative
- Dropping the `home.trending` placement (C3-U9): do it inside recsys-cart-cross-sell only if it is still declared, as config deletion.
- Doc drift in `MASTER_CAPABILITY_MATRIX.md` and the other docs (Q38).

## NFR and data
- NFR:
  - `SearchListings` p95 with a sort or rating filter must not regress more than 10% against the current baseline on the local stack.
  - Sold count and rating reach the search document within 30 s of the source event.
  - `generateMetadata` adds no extra gateway call beyond the page's own cached loader.
  - `sitemap.xml` renders in under 2 s for 5,000 listings.
- Personal data: none new. `request_id` identifies a query, not a person. Beacons already carry the user/anonymous id under the existing consent rules. `ListingSalesChanged` and `ListingRatingChanged` carry no buyer or reviewer ids.
- Risk routing expected: `proto` (contract). No auth, payment or pii routing.

## Conventions
- Mirror `RecommendationsRow.tsx` for impression and click attribution.
- Mirror `RecommendationPerformance` for the analytics query.
- Mirror the team-order outbox for `ListingSalesChanged`.
- The team-search indexer consumer pattern is `consumer.NewListingEventHandler`.
- e2e lives in `platform-e2e/tests/e2e/features/<area>/`. Rewrite `search_rating_removed.feature` and the "no Bán Chạy" assertion in `search_facets.feature:39`; do not delete them.
- No comments in code or config diffs. Every service image runs as non-root. No `*_ENABLED` flags for the new consumers.

## Locked
2026-11-07 — Confirmed by Bùi Đăng Phúc. Brand "Agora"; sold_count = paid minus cancelled-from-PAID or refunded, owned by team-order; rating floors 3/4 only, no top-rated sort; Q32 implements strategies and filters and renders cart cross-sell; owners emit absolute values (D1), backfill on migration (D2), min_rating re-enabled (D3).

## Clarifications (2026-11-07, round 1 review, answered by Bùi Đăng Phúc)
- C1 `context_listing_ids` over 50 → gateway and team-ai both return INVALID_ARGUMENT; the cart page sends at most the 50 newest lines.
- C2 `ReturnRefunded` reverses the whole order's quantity (returns are whole-order today); partial refunds wait for line-level returns.
- C3 `ListingRatingChanged` envelopes carry no principal (live and backfill alike); no new personal data reaches team-search.
- C4 Sitemap shop URLs come from new `SearchHit.seller_id` (contract slice); frontend-seo depends on discovery-seo-contract.
- C5 Concurrent review writes: `pg_advisory_xact_lock` per listing_id before reading the aggregate in the writing transaction.
- C6 Pending search docs (values for a listing not yet indexed) are purged by the existing sweep once older than TOMBSTONE_TTL; delta on search-read-model-deletes.
- C7 The ratings facet ignores its own `min_rating` filter (other filters still apply).
- C8 frontend-seo also covers team-frontend-seller and team-frontend-admin: root layout `robots: noindex,nofollow` and title template `%s | Agora`, hand-written suffixes removed.
- C9 Orchestrator calls (no human needed): `GetSearchPerformance` added to the contract; the frontend part of search-bestseller-rating moves to new slice frontend-discovery-sort-rating (ownership); latency NFRs are measured, non-gating unless a scenario names them; migrations deploy together with the new writer binary; search mapping changes are additive and applied by the indexer at startup; `assistant` stays indexable; listings without image omit og:image; shops use `banner_url` for og:image; brand-string hygiene covers `.ts`/`.tsx` sources of public pages.
- C10 (human, 2026-11-07) team-ai candidates have no seller/category/visibility today: a new producer slice `recsys-candidate-attributes` (owner platform-recsys) adds `seller_id` and listing `status` to featurestore `item_attributes`, and the platform-recsys loader writes `seller_id`, `category_id` and `visible` into the Qdrant point payload and the Redis popular/user cache entries. No request-time enrichment call.
- C11 (orchestrator) Sitemap: `SearchListings` caps pages at 50 with offset cursors (100 sequential calls for 5,000), so the sitemap is served from the Next.js cache (`revalidate = 3600`); the cold-render time is measured, non-gating. Unknown shop id renders 200 with the fallback name (no new RPC).
- C12 (orchestrator) COD orders count toward `sold_count` only once team-order emits `OrderPaidEvent` for them; listed as NEEDS_HUMAN in LOCK.md.
- C13 (human, 2026-11-07) `seller_id` is already in `item_attributes@v1`; only listing status is missing. New slice `analytics-listing-status-export` (owner team-analytics) adds a `status` column to the `listing_sellers.parquet` export from `ListingChanged`; `visible = (status == LISTING_STATUS_PUBLISHED)`. recsys-candidate-attributes depends on it.
- C14 (human, 2026-11-07) No source writes `in_stock` today (team-ai defaults it to true), so `in_stock_only` stays a documented no-op in this initiative; a real stock source is C3-U1, follow-up.
- C15 (human, 2026-11-07) Status backfill: team-analytics gets a one-off replay command that re-reads `listing.events` from the start on its own consumer group and fills `status` for every listing; run once at deploy. A DELETED change type exports `status` null (no DELETED enum exists; fails closed) — orchestrator call, NEEDS_HUMAN confirm at lock.
- C16 (human, 2026-11-07) Stale `item_attributes@v2`: the recsys loader logs a warning with the snapshot `as_of` when it is older than 24 h and still publishes; `visible` lags a moderation change by at most one export + featurestore run + publish/backfill cycle.
