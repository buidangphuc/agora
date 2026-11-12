# 00-spec — discovery follow-ups: COD sales, DELETED status, in-stock recommendations

initiative: discovery-followups

## Goal
Three gaps left open by `discovery-seo` (plans/discovery-seo/LOCK.md "Open issues") are closed:
COD orders count toward "Bán chạy" (`sold_count`) once completed; a deleted listing carries an
explicit `DELETED` status in the analytics export and item attributes instead of null; and
recommendation placements that ask for `in_stock_only` really drop sold-out listings, using live
stock from team-search.

## User stories
1. **COD counts when completed.** Given a COD order for listing L in SHIPPED, when it moves to
   COMPLETED (buyer, seller or auto-complete sweep), then L's `sold_count` rises by the order's
   quantity and team-search shows the new value (via `ListingSalesChanged`).
   Edge: an online-paid order that completes is not counted twice (it was counted at PAID).
   Edge: historical COD orders already COMPLETED are backfilled once.
2. **Deleted listing has a status.** Given listing L is deleted (team-domain emits
   `ListingChanged(CHANGE_TYPE_DELETED)`), when team-analytics projects it, then L's exported
   `status` is `DELETED`, and `item_attributes@v2` / the recsys payload mark it not visible.
   Edge: `listing-replay` re-derives `DELETED` for listings deleted before this change.
3. **In-stock recommendations.** Given a placement with `in_stock_only` and a candidate whose
   search stock is 0, when team-ai ranks, then that candidate is not returned.
   Edge: team-search unavailable or slow → team-ai drops all candidates of that placement
   (fail closed) and the placement is empty.

## Non-goals
- No new COD payment, collection or wallet flow; `OrderPaidEvent` is not emitted for COD
  (team-payment would credit the seller wallet).
- No reversal of `sold_count` for returned COD orders (no COD refund path exists) — known gap.
- No restore of deleted listings; team-domain keeps hard delete; the `ListingChanged` payload is unchanged.
- No change to search's own `in_stock` filter or read model.
- No staging/prod deploy work (gitops topics, runbooks) — local first.

## Architecture decisions
- COD sales are counted in team-order inside the existing `UpdateOrderStatusFrom` compare-and-set
  on SHIPPED→COMPLETED when the order's payment method is COD (`countOrderSales`), and published
  through the existing `ListingSalesChanged` outbox event. Why: one place already owns the count;
  no new event, no wallet side effect.
- Backfill of COD COMPLETED orders is a team-order migration, idempotent through
  `listing_sales_orders`. Why: mirrors migration 0023's paid-order backfill.
- `LISTING_STATUS_DELETED = 7` is added to `ListingStatus`; team-analytics writes it when the
  change type is DELETED (live consumer and `listing-replay`). team-domain events are unchanged.
  platform-featurestore accepts `DELETED` as a known status; platform-recsys keeps
  `visible = status == PUBLISHED` (so `false`, not absent). Why: explicit status, no consumer of
  team-domain events has to handle a new value.
- Stock for recommendations is read live from team-search by a new RPC (C3-U1, plans/ai-first/DECISIONS.md:52),
  at most 10 listing ids per call; a listing is in stock when its projected stock is > 0
  (the listing's projected base stock, net of reservations, exactly as search's `in_stock` filter and `SearchHit.stock`; no variant rollup).
  team-ai fails closed on error, timeout or unknown stock. Why: batch attributes would be up to 24 h stale.
- The new RPC is service-to-service only; team-gateway does not expose it.

## Contracts
```proto
// platform/listing/v1/listing.proto — ListingStatus
LISTING_STATUS_DELETED = 7;   // set by read models for a hard-deleted listing; team-domain never returns it

// platform/search/v1/search.proto — add to SearchService
rpc GetListingStock(GetListingStockRequest) returns (GetListingStockResponse);

message GetListingStockRequest {
  repeated string listing_ids = 1;   // 1..10, else INVALID_ARGUMENT
}
message ListingStock {
  string listing_id = 1;
  optional int32 stock = 2;          // absent = not projected / unknown
}
message GetListingStockResponse {
  repeated ListingStock listings = 1; // one entry per requested id, request order
}
```
Existing contracts consumed unchanged: `order.events/ListingSalesChanged`,
`listing.events/ListingChanged`, `listing.events/ListingStockChanged`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contracts | discovery-followups-contract | platform-core | – | provides: SearchService.GetListingStock, ListingStatus DELETED | platform-core |
| COD orders count toward sold_count | order-cod-sales-count | team-order | – | uses: order.events/ListingSalesChanged | team-order, platform-e2e |
| deleted listing exported as DELETED | analytics-listing-deleted-status | team-analytics | discovery-followups-contract | uses: listing.events/ListingChanged | team-analytics, platform-featurestore, platform-recsys, platform-e2e |
| live stock lookup by listing ids | search-listing-stock-lookup | team-search | discovery-followups-contract | provides impl: SearchService.GetListingStock | team-search, platform-e2e |
| in_stock_only drops sold-out recommendations | recs-in-stock-filter | team-ai | search-listing-stock-lookup | uses: SearchService.GetListingStock | team-ai, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `docker-compose.yaml` / team-ai upstream env (search address) — recs-in-stock-filter only; none shared.
- `platform-e2e/FEATURES` steps for recsys — analytics-listing-deleted-status and recs-in-stock-filter may both touch
  `platform-e2e/tests/e2e/flows/item_attributes_support.py`.

## Out of initiative
- Deploy-only items from discovery-seo (gitops `search-indexer.*.dlq` topics, listing-replay runbook,
  recsys deploy order, `NEXT_PUBLIC_SITE_URL` build arg) — deferred, local first.

## NFR and data
- NFR: `GetListingStock` p95 ≤ 20 ms locally for 10 ids; team-ai stock call within its existing
  retrieve budget, timeout → fail closed. COD backfill migration runs once and is idempotent.
- Personal data: none (listing ids, stock counts, order quantities).
- Risk routing expected: `proto` (contract slice). No auth/payment/pii: COD counting touches no money movement.

## Conventions
- Mirror `countOrderSales` / `reverseOrderSales` in team-order/internal/store/order.go and
  migration 0023 for the backfill.
- team-analytics: team-analytics/internal/events/listing.go (status blanking today) and
  internal/app/listing_replay.go.
- team-search RPC: mirror SearchListings handler style and `stock` projection (internal/index/opensearch.go).
- team-ai: `ranking/filter.py` `in_stock_only`; candidates default `in_stock=True` in
  adapters/qdrant.py and redis_cache.py today — that default must not leak through.
- Tests: Go Docker-backed tests need `GO_SDK_REQUIRE_DOCKER=1`; recsys e2e drivers write v2 snapshots
  via `tests/e2e/flows/item_attributes_support.py`.

## Locked
2026-11-12 — Confirmed by Bùi Đăng Phúc. COD counts at COMPLETED in team-order (+ backfill, no COD return reversal); LISTING_STATUS_DELETED set by team-analytics (+ replay); team-ai in_stock_only via live SearchService.GetListingStock (≤10 ids, stock>0, fail closed).
Clarified 2026-11-12 (spec review round 1, Bùi Đăng Phúc): in stock = projected base stock > 0 (no variant rollup); a missing/absent entry drops only that candidate, while a call error, timeout or malformed response empties the placement; team-ai looks up stock only for the top ranked candidates (limit × 2, capped at 20, ≤ 2 calls) and refills from the next ranked ones within the budget; COD completion p95 rises ≤ 5 ms; DELETED is terminal in team-analytics.
