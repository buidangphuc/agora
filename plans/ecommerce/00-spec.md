# 00-spec — 4 ecommerce features (parallel, e2e each)

> Self-contained spec. Copied into EVERY sub-agent prompt. Contracts here are the source of truth; agents code against them, never against each other's code.

## Goal
Ship 4 ecommerce features into the marketplace polyrepo, in conflict-free parallel waves, each with automated e2e:
1. **Voucher & Flash Sale engine** — new service `team-promotion` (:50061).
2. **Faceted search + filters** — facet aggregations on `team-search`.
3. **Wishlist collections + price/stock alerts** — `team-engagement` (collections) + `team-notification` (alerts).
4. **Richer reviews** — media, helpful votes, verified-purchase, shop rating rollup on `team-engagement`.

Done = all 4 merge, every shared file changed once in Wave 0, no two same-wave tasks share a write-set, per-repo `go test` + `buf lint/breaking` green, platform-e2e suites green.

## Non-goals
- No real money movement (§7): voucher/flash-sale are discount **calculation** only; payment/wallet stay mock.
- No cross-DB joins (Rule 3): need another domain's data → call its gRPC.
- No new CI pipeline; ride existing `deploy/act` → ArgoCD flow.
- No refactor of unrelated code; no proto renumber/removal (additive only, ADR-0001).
- Flash sale scope = time-boxed sale price + capped stock + live-stock read RPC. NOT a separate auction/bidding system.

## Architecture rules (MUST NOT break)
- Proto changed in `platform-core/packages/proto` FIRST, additive, `buf lint`+`buf breaking` pass; then re-vendor + `buf generate` into each consumer. Never hand-edit generated code.
- Frontend → Gateway only; Gateway routes/forwards, holds no business logic.
- Each service owns its DB. New bounded context = new repo (`team-promotion`), copy team-order's shape.
- Redemption durability (ADR-0007/8/9): saga state persisted before external effects; `ValidateAndReserve` idempotent on `reservation_id`; settle event-carried via outbox; consumers commit offset only after handler success.
- Right broker (ADR-0002): state-change events → Kafka (`promotion.events`); jobs → RabbitMQ.

## Contracts

### F1 — NEW `platform/promotion/v1/promotion.proto` (team-promotion :50061)
```proto
syntax = "proto3";
package platform.promotion.v1;
import "google/protobuf/timestamp.proto";
import "platform/common/v1/common.proto";

// Discounts only — never money movement (§7). Owned by team-promotion (own DB).
service VoucherService {
  rpc CreateVoucher(CreateVoucherRequest) returns (Voucher);          // seller/platform admin
  rpc GetVoucher(GetVoucherRequest) returns (Voucher);
  rpc ListVouchers(ListVouchersRequest) returns (ListVouchersResponse);
  // Checkout seam — mirrors ReserveStock (ADR-0008): idempotent on reservation_id.
  rpc ValidateAndReserve(ValidateAndReserveRequest) returns (ValidateAndReserveResponse);
  rpc CommitReservation(CommitReservationRequest) returns (CommitReservationResponse);
  rpc ReleaseReservation(ReleaseReservationRequest) returns (ReleaseReservationResponse);
}
service FlashSaleService {
  rpc CreateCampaign(CreateCampaignRequest) returns (FlashSaleCampaign);
  rpc GetActiveFlashSale(GetActiveFlashSaleRequest) returns (GetActiveFlashSaleResponse); // by listing_id
  rpc ListActiveCampaigns(ListActiveCampaignsRequest) returns (ListActiveCampaignsResponse);
  // Live remaining stock for the flash-sale banner/meter (frontend LiveFlashSaleStock).
  rpc GetFlashSaleStock(GetFlashSaleStockRequest) returns (GetFlashSaleStockResponse);
}
enum DiscountType { DISCOUNT_TYPE_UNSPECIFIED = 0; DISCOUNT_TYPE_PERCENT = 1; DISCOUNT_TYPE_FIXED = 2; }
enum VoucherScope { VOUCHER_SCOPE_UNSPECIFIED = 0; VOUCHER_SCOPE_SHOP = 1; VOUCHER_SCOPE_PLATFORM = 2; }
message Voucher {
  string id = 1; string code = 2; VoucherScope scope = 3; string seller_id = 4;
  DiscountType discount_type = 5; int64 discount_value = 6;   // percent (1-100) or minor-unit amount
  int64 min_spend = 7; int64 max_discount = 8; int64 quota = 9; int64 used = 10;
  google.protobuf.Timestamp starts_at = 11; google.protobuf.Timestamp ends_at = 12;
}
message CreateVoucherRequest { string code = 1; VoucherScope scope = 2; DiscountType discount_type = 3;
  int64 discount_value = 4; int64 min_spend = 5; int64 max_discount = 6; int64 quota = 7;
  google.protobuf.Timestamp starts_at = 8; google.protobuf.Timestamp ends_at = 9; }
message GetVoucherRequest { string code = 1; }
message ListVouchersRequest { string seller_id = 1; platform.common.v1.PageRequest page = 2; }
message ListVouchersResponse { repeated Voucher vouchers = 1; platform.common.v1.PageResponse page = 2; }
message ValidateAndReserveRequest { string reservation_id = 1; string code = 2; string buyer_id = 3;
  int64 cart_subtotal = 4; string seller_id = 5; }
message ValidateAndReserveResponse { bool valid = 1; string reason = 2; int64 discount_amount = 3;
  string voucher_id = 4; }
message CommitReservationRequest { string reservation_id = 1; }
message CommitReservationResponse { bool committed = 1; }
message ReleaseReservationRequest { string reservation_id = 1; }
message ReleaseReservationResponse { bool released = 1; }
message FlashSaleCampaign { string id = 1; string listing_id = 2; string variant_id = 3;
  int64 sale_price = 4; int64 stock_cap = 5; int64 stock_sold = 6;
  google.protobuf.Timestamp starts_at = 7; google.protobuf.Timestamp ends_at = 8; }
message CreateCampaignRequest { string listing_id = 1; string variant_id = 2; int64 sale_price = 3;
  int64 stock_cap = 4; google.protobuf.Timestamp starts_at = 5; google.protobuf.Timestamp ends_at = 6; }
message GetActiveFlashSaleRequest { string listing_id = 1; }
message GetActiveFlashSaleResponse { bool active = 1; FlashSaleCampaign campaign = 2; }
message ListActiveCampaignsRequest { platform.common.v1.PageRequest page = 1; }
message ListActiveCampaignsResponse { repeated FlashSaleCampaign campaigns = 1; platform.common.v1.PageResponse page = 2; }
message GetFlashSaleStockRequest { string campaign_id = 1; }
message GetFlashSaleStockResponse { int64 remaining = 1; int64 stock_cap = 2; }
```
Kafka: emit `promotion.events` (EventEnvelope) on voucher/campaign create/update. Redemption is **sync gRPC** in the order saga, not fire-and-forget.

### F1 redemption on order — additive to `platform/order/v1/order.proto`
- `CreateOrderRequest`: add `string voucher_code = 4;`
- `Order`: add `int64 discount_amount = 15;` `string voucher_code = 16;` (`total_amount = items_subtotal + shipping_fee - discount_amount`, floor 0)

### F2 — additive to `platform/search/v1/search.proto`
Request already has `min_price`/`max_price`/`min_rating`/`category_id`. Add facet output only:
```proto
message FacetBucket { string key = 1; int64 count = 2; }
message Facets {
  repeated FacetBucket categories = 1;    // key = category_id
  repeated FacetBucket price_ranges = 2;  // key = "0-100000" bucket label
  repeated FacetBucket ratings = 3;       // key = "4" (>=4 stars) etc.
  repeated FacetBucket sellers = 4;       // key = seller_id
}
// add to SearchListingsResponse:  Facets facets = 3;
```

### F3 — additive to `platform/engagement/v1/engagement.proto` (wishlist collections)
```proto
// add RPCs to EngagementService:
rpc CreateCollection(CreateCollectionRequest) returns (Collection);
rpc ListCollections(ListCollectionsRequest) returns (ListCollectionsResponse);
rpc AddToCollection(AddToCollectionRequest) returns (AddToCollectionResponse);
rpc RemoveFromCollection(RemoveFromCollectionRequest) returns (RemoveFromCollectionResponse);
rpc ListCollectionItems(ListCollectionItemsRequest) returns (ListCollectionItemsResponse);
message Collection { string id = 1; string user_id = 2; string name = 3; int64 item_count = 4;
  google.protobuf.Timestamp created_at = 5; }
message CreateCollectionRequest { string name = 1; }
message ListCollectionsRequest {}
message ListCollectionsResponse { repeated Collection collections = 1; }
message AddToCollectionRequest { string collection_id = 1; string listing_id = 2; }
message AddToCollectionResponse {}
message RemoveFromCollectionRequest { string collection_id = 1; string listing_id = 2; }
message RemoveFromCollectionResponse {}
message ListCollectionItemsRequest { string collection_id = 1; platform.common.v1.PageRequest page = 2; }
message ListCollectionItemsResponse { repeated string listing_ids = 1; platform.common.v1.PageResponse page = 2; }
```

### F4 — additive to `platform/engagement/v1/engagement.proto` (richer reviews)
```proto
// add to Review message:
repeated string media_urls = 9; int64 helpful_count = 10; bool verified_purchase = 11;
// add to CreateReviewRequest:  repeated string media_urls = 5;
// add RPCs:
rpc MarkReviewHelpful(MarkReviewHelpfulRequest) returns (MarkReviewHelpfulResponse);
rpc GetShopRatingSummary(GetShopRatingSummaryRequest) returns (GetShopRatingSummaryResponse);
message MarkReviewHelpfulRequest { string review_id = 1; }
message MarkReviewHelpfulResponse { int64 helpful_count = 1; }
message GetShopRatingSummaryRequest { string seller_id = 1; }
message GetShopRatingSummaryResponse { string seller_id = 1; double average_rating = 2;
  int64 review_count = 3; RatingBreakdown breakdown = 4; }
```
`verified_purchase` = check buyer has a delivered order for the listing via team-order gRPC (no DB join).

### F3 alerts — additive to `platform/notification/v1/notification.proto`
```proto
// add RPCs to NotificationService:
rpc SubscribeAlert(SubscribeAlertRequest) returns (SubscribeAlertResponse);
rpc UnsubscribeAlert(UnsubscribeAlertRequest) returns (UnsubscribeAlertResponse);
rpc ListAlertSubscriptions(ListAlertSubscriptionsRequest) returns (ListAlertSubscriptionsResponse);
enum AlertType { ALERT_TYPE_UNSPECIFIED = 0; ALERT_TYPE_PRICE_DROP = 1; ALERT_TYPE_BACK_IN_STOCK = 2; }
// add NotificationType values (additive): NOTIFICATION_TYPE_PRICE_DROP = 5; NOTIFICATION_TYPE_BACK_IN_STOCK = 6;
message AlertSubscription { string id = 1; string user_id = 2; string listing_id = 3; AlertType type = 4; }
message SubscribeAlertRequest { string listing_id = 1; AlertType type = 2; }
message SubscribeAlertResponse { AlertSubscription subscription = 1; }
message UnsubscribeAlertRequest { string subscription_id = 1; }
message UnsubscribeAlertResponse {}
message ListAlertSubscriptionsRequest {}
message ListAlertSubscriptionsResponse { repeated AlertSubscription subscriptions = 1; }
```
team-notification consumes `listing.events` (pricing/stock changed) → matches subscriptions → creates notifications. Subscriptions owned by team-notification (engagement stays collections-only).

## Conventions
- Go: `gofmt` + `go vet` + `go test ./...` clean; tests colocated `internal/*/*_test.go`; gRPC tests in-process on ephemeral ports; repo tests cover Postgres + in-memory impls. All Go/buf runs in Docker (host has no Go/buf).
- Migration numbers (assigned, do NOT collide): promotion `0001*`; order `0005_voucher_redemption`; engagement `0004_wishlist_collections` + `0005_review_enrichment`; notification `002_alert_subscriptions`; search = no SQL (OpenSearch mapping in repo code).
- Feature flags: OpenFeature+Flipt per-repo; add `Flag*` const, evaluate at handler, fail-open. Pattern: `team-order/internal/featureflags`.
- e2e: platform-e2e pytest-bdd + Playwright; `.feature` + steps + page objects per feature; flip status in `platform-e2e/FEATURES.yaml` AND owning repo `FEATURES.yaml`.
- Commit trailer: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

## Reference implementation (copy, don't invent)
- New service skeleton: copy `team-order` shape (`cmd/server/main.go`, `internal/{config,bootstrap,grpcserver,handler,service,repository,interceptor,featureflags}`, `migrations/`, `buf.gen.yaml`, `.env.example`).
- Cleanest CRUD slice to mirror: `team-order/internal/{handler,service,repository}/cart.go` (+ `cart_test.go`).
- Saga + upstream + Kafka consumer: `team-order/internal/service/saga.go`, `internal/upstream/domain.go`, `internal/consumer/payment.go`.
- Gateway forwarder: `team-gateway/internal/edge/order.go` + `edge/server.go` + `upstream/*.go`.

## Locked
2026-09-03. Confirmed by user: all 4 features; promotion = FULL (voucher + flash sale); fan-out = subagents 1 session; each feature has e2e. Assumptions locked: team-promotion is a new service at :50061 (AGENTS.md §6c); redemption is sync reserve→commit/release in the order saga, idempotent on reservation_id; voucher/flash-sale are discount calculation only (payment stays mock, §7); alert subscriptions owned by team-notification (no cross-domain write); search facets are additive-only (request filters already exist). No open questions remain.
