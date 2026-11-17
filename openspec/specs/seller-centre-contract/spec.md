# seller-centre-contract Specification

## Purpose
Defines the additive wire contract of the seller-centre initiative: the seller-scoped promotion RPCs, `CartService.AddBundleToCart` with bundle fields on cart and order lines, and the paging, query and status filters on the seller order and listing lists.

## Requirements

### Requirement: Promotion exposes seller-scoped campaign and shop voucher RPCs

`FlashSaleService` in `platform/promotion/v1/promotion.proto` SHALL gain `ListMyCampaigns(ListMyCampaignsRequest) returns (ListMyCampaignsResponse)` and `CancelMyCampaign(CancelMyCampaignRequest) returns (CancelMyCampaignResponse)`; `VoucherService` SHALL gain `ListMyShopVouchers(ListMyShopVouchersRequest) returns (ListMyShopVouchersResponse)` and `DeactivateMyShopVoucher(DeactivateMyShopVoucherRequest) returns (DeactivateMyShopVoucherResponse)`. Messages: `ListMyCampaignsRequest {statuses=1 repeated CampaignStatus, page=2 PageRequest}`, `ListMyCampaignsResponse {campaigns=1 repeated FlashSaleCampaign, page=2 PageResponse}`, `CancelMyCampaignRequest {campaign_id=1}`, `CancelMyCampaignResponse {campaign=1}`, `ListMyShopVouchersRequest {page=1}`, `ListMyShopVouchersResponse {vouchers=1 repeated Voucher, page=2}`, `DeactivateMyShopVoucherRequest {voucher_id=1}`, `DeactivateMyShopVoucherResponse {voucher=1}`. `FlashSaleCampaign` SHALL gain `string seller_id = 11`, commented server-owned. The RPC comments SHALL state: the seller comes from the principal and never from a request field; `listing.write` is required; a foreign or unknown id gives `PERMISSION_DENIED`; `CancelMyCampaign` accepts only a SCHEDULED campaign and gives `FAILED_PRECONDITION` otherwise; `DeactivateMyShopVoucher` is idempotent; neither mutation is audited. Existing RPCs, messages and fields SHALL NOT change.

#### Scenario: Seller centre contract: Seller promotion RPCs exist
- **WHEN** the compiled `FlashSaleService` and `VoucherService` descriptors are inspected
- **THEN** every existing RPC is unchanged and the four new RPCs exist with the request and response types and field numbers listed above, and no request message has a seller id field
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_SellerPromotionRPCsExist`

#### Scenario: Seller centre contract: FlashSaleCampaign carries seller_id
- **WHEN** the descriptor test reads `FlashSaleCampaign`
- **THEN** fields 1 to 10 equal the committed expectations and `seller_id`=11 is a string
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_FlashSaleCampaignCarriesSellerID`

#### Scenario: Seller centre contract: Seller promotion authz and preconditions are documented
- **WHEN** a reader opens the comments of the four RPCs to learn what a caller who does not own the campaign or voucher gets
- **THEN** they contain "principal", `listing.write`, `PERMISSION_DENIED` for a foreign or unknown id, `FAILED_PRECONDITION` and "SCHEDULED" for `CancelMyCampaign`, "idempotent" for `DeactivateMyShopVoucher` and "not audited"
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_SellerPromotionAuthzAndPreconditionsDocumented`

#### Scenario: Seller centre contract: Missing authz note is caught
- **WHEN** the test's comment helper runs against a fixture `CancelMyCampaign` comment lacking `PERMISSION_DENIED`
- **THEN** the helper reports failure (the test expects it), proving a missing note is rejected
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_MissingAuthzNoteIsCaught`

#### Scenario: Seller centre contract: Campaign and voucher lists round-trip
- **WHEN** a `ListMyCampaignsResponse` with two campaigns (one with `seller_id` set, `stock_sold` int64 max) and a `ListMyShopVouchersResponse` with one voucher and `page.next_cursor` "c1" are serialised and parsed
- **THEN** every field parses back equal, and a `FlashSaleCampaign` without `seller_id` parses back with it empty
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_CampaignAndVoucherListsRoundTrip`

#### Scenario: Seller centre contract: Old readers skip seller_id
- **WHEN** bytes of a `FlashSaleCampaign` carrying field 11 are parsed with a schema that knows only fields 1-10
- **THEN** parsing succeeds, fields 1-10 are intact and field 11 is skipped without error
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/sellercentre_compat_test.go` `TestSellerCentre_OldReadersSkipSellerID`

### Requirement: CartService exposes AddBundleToCart and bundle fields

`CartService` in `platform/order/v1/order.proto` SHALL gain `AddBundleToCart(AddBundleToCartRequest) returns (AddBundleToCartResponse)` with `AddBundleToCartRequest {bundle_id=1 string, quantity=2 int32}` and `AddBundleToCartResponse {cart=1 Cart}`. `CartItem` SHALL gain `string bundle_id = 10` and `int64 bundle_discount = 11`; `Cart` SHALL gain `int64 bundle_discount_total = 4`; `OrderItem` SHALL gain `string bundle_id = 9` and `int64 bundle_discount = 10`; existing fields SHALL NOT change. The `AddBundleToCart` comment SHALL state: one cart line per bundle listing tagged with `bundle_id`; `FAILED_PRECONDITION` when a listing is unpublished, out of stock, not owned by the bundle seller or a variant listing; `NOT_FOUND` for no bundle; `INVALID_ARGUMENT` for `quantity` below 1. The field comments SHALL state that the bundle fields are server-owned and ignored in requests, and that `bundle_discount` is the pro-rata share of the bundle discount for that line in VND.

#### Scenario: Seller centre contract: AddBundleToCart RPC exists
- **WHEN** the compiled `CartService` descriptor is inspected
- **THEN** every existing RPC is unchanged plus `AddBundleToCart` with `AddBundleToCartRequest` (`bundle_id`=1 string, `quantity`=2 int32) and `AddBundleToCartResponse` (`cart`=1 `Cart`)
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_AddBundleToCartRPCExists`

#### Scenario: Seller centre contract: Bundle fields have fixed numbers
- **WHEN** the descriptor test reads `CartItem`, `Cart` and `OrderItem`
- **THEN** `CartItem` fields 1-9 and `Cart` fields 1-3 and `OrderItem` fields 1-8 equal the committed expectations, and `CartItem.bundle_id`=10 (string), `CartItem.bundle_discount`=11 (int64), `Cart.bundle_discount_total`=4 (int64), `OrderItem.bundle_id`=9 (string), `OrderItem.bundle_discount`=10 (int64) exist
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_BundleFieldsHaveFixedNumbers`

#### Scenario: Seller centre contract: AddBundleToCart failures are documented
- **WHEN** a reader opens the `AddBundleToCart` comment to learn the error cases for a bundle that cannot be bought
- **THEN** it contains `FAILED_PRECONDITION` with "unpublished", "out of stock", "not owned by the bundle seller" and "variant", `NOT_FOUND` for no bundle and `INVALID_ARGUMENT` for quantity below 1
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_AddBundleToCartFailuresAreDocumented`

#### Scenario: Seller centre contract: Bundle fields are documented server-owned
- **WHEN** a reader opens the comments of `CartItem.bundle_discount` and `OrderItem.bundle_discount`
- **THEN** they contain "server-owned" and "pro-rata"
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_BundleFieldsAreDocumentedServerOwned`

#### Scenario: Seller centre contract: Cart with bundle lines round-trips
- **WHEN** a `Cart` with two items tagged `bundle_id` "b1" with `bundle_discount` 3333 and 3334 and `bundle_discount_total` 6667 is serialised and parsed, and an `OrderItem` with `bundle_discount` int64 max likewise
- **THEN** every field parses back equal, and a `Cart` without bundle fields parses back with `bundle_id` empty and all discounts 0
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_CartWithBundleLinesRoundTrips`

#### Scenario: Seller centre contract: Old readers skip bundle fields
- **WHEN** bytes of a `Cart` and an `OrderItem` carrying the new fields are parsed with the previous schema
- **THEN** parsing succeeds, the old fields are intact and the new ones are skipped without error
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_OldReadersSkipBundleFields`

### Requirement: ListSellerOrders gains optional paging, search and counts

`ListSellerOrdersRequest` SHALL gain `PageRequest page = 2`, `string query = 3` and `repeated OrderStatus statuses = 4`, keeping `status_filter = 1`. `ListSellerOrdersResponse` SHALL gain `PageResponse page = 2` and `repeated OrderStatusCount status_counts = 3`, with new `message OrderStatusCount {OrderStatus status = 1; int64 count = 2;}`. Comments SHALL state: an unset `page` returns today's full list; `query` is an order id prefix or item title substring of at most 100 characters; `statuses` is an OR filter that wins over `status_filter`; `status_counts` covers all the calling seller's orders with `query` applied and `statuses` ignored; an unknown cursor gives `INVALID_ARGUMENT`; the seller comes from the principal.

#### Scenario: Seller centre contract: ListSellerOrders fields have fixed numbers
- **WHEN** the descriptor test reads `ListSellerOrdersRequest`, `ListSellerOrdersResponse` and `OrderStatusCount`
- **THEN** `status_filter`=1, `orders`=1 are unchanged and `page`=2 (`PageRequest`), `query`=3 (string), `statuses`=4 (repeated `OrderStatus`), response `page`=2 (`PageResponse`), `status_counts`=3 (repeated `OrderStatusCount`), `OrderStatusCount.status`=1 and `count`=2 (int64) exist
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_ListSellerOrdersFieldsHaveFixedNumbers`

#### Scenario: Seller centre contract: ListSellerOrders semantics are documented
- **WHEN** a reader opens the comments of the new fields to learn the behaviour for an unset page and for a bad cursor
- **THEN** they contain "unset", "full list", "100", "wins over", "status_counts", "query applied", `INVALID_ARGUMENT` and "principal"
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_ListSellerOrdersSemanticsAreDocumented`

#### Scenario: Seller centre contract: Unpaged request keeps the old wire form
- **WHEN** a `ListSellerOrdersRequest` with only `status_filter` set and a `ListSellerOrdersResponse` with only `orders` set are serialised
- **THEN** the bytes equal those of the previous schema and parse back with `page` unset, `query` empty and `status_counts` empty
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_UnpagedRequestKeepsTheOldWireForm`

#### Scenario: Seller centre contract: Paged response round-trips
- **WHEN** a `ListSellerOrdersResponse` with `page.next_cursor` "c2", `page.total` -1 and three `status_counts` (count 5000 among them) is serialised and parsed
- **THEN** every field parses back equal
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/sellercentre_compat_test.go` `TestSellerCentreContract_PagedResponseRoundTrips`

### Requirement: ListMyListings gains optional status and title filters

`ListMyListingsRequest` SHALL gain `repeated ListingStatus statuses = 2` and `string query = 3`, keeping `page = 1`. Comments SHALL state: empty `statuses` means all; `query` is a case-insensitive title substring of at most 100 characters; an empty request returns what it returned before; the seller comes from the principal; an unknown cursor gives `INVALID_ARGUMENT`. `ListMyListingsResponse` SHALL NOT change.

#### Scenario: Seller centre contract: ListMyListings filter fields exist
- **WHEN** the descriptor test reads `ListMyListingsRequest`
- **THEN** `page`=1 is unchanged and `statuses`=2 (repeated `ListingStatus`) and `query`=3 (string) exist, and `ListMyListingsResponse` still has exactly `listings`=1 and `page`=2
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/sellercentre_compat_test.go` `TestSellerCentre_ListMyListingsFilterFieldsExist`

#### Scenario: Seller centre contract: ListMyListings filter semantics are documented
- **WHEN** a reader opens the comments of `statuses` and `query`
- **THEN** they contain "empty", "all", "case-insensitive", "substring", "100" and `INVALID_ARGUMENT` for an unknown cursor
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/sellercentre_compat_test.go` `TestSellerCentre_ListMyListingsFilterSemanticsAreDocumented`

#### Scenario: Seller centre contract: Empty filter request keeps the old wire form
- **WHEN** a `ListMyListingsRequest` with only `page.page_size` 50 is serialised
- **THEN** the bytes equal those of the previous schema, and a request with two statuses and `query` of 100 characters round-trips equal
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/sellercentre_compat_test.go` `TestSellerCentre_EmptyFilterRequestKeepsTheOldWireForm`

### Requirement: Storefront validation rules are documented in the contract

The comments of `Storefront` and `UpsertStorefrontRequest` in `platform/listing/v1/listing.proto` SHALL state: `tagline` is at most 120 runes with no control characters; `banner_url` is an object URL from the listing-images bucket obtained through `GetImageUploadUrl`, or empty; `featured_listing_ids` holds at most 6 ids, unique, owned by the caller and existing, otherwise `INVALID_ARGUMENT` and nothing is saved; `theme` is one of `default`, `dark`, `festive`; the request carries the full storefront. No field, number or type of `Storefront` SHALL change.

#### Scenario: Seller centre contract: Storefront fields are unchanged
- **WHEN** the descriptor test reads `Storefront`
- **THEN** fields 1 to 7 (`seller_id`, `slug`, `banner_url`, `tagline`, `featured_listing_ids`, `theme`, `display_name`) keep their names, numbers and types and no field 8 or higher exists
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/sellercentre_compat_test.go` `TestSellerCentre_StorefrontFieldsAreUnchanged`

#### Scenario: Seller centre contract: Storefront rules are documented
- **WHEN** a reader opens the comments to learn which storefront input is invalid
- **THEN** they contain "120", "control", "listing-images", "GetImageUploadUrl", "6", "unique", "owned by the caller", `INVALID_ARGUMENT`, "nothing is saved", `default`, `dark` and `festive`
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/sellercentre_compat_test.go` `TestSellerCentre_StorefrontRulesAreDocumented`

### Requirement: The change is backward compatible, generated and copied to consumers

The proto change SHALL be additive: `make -C platform-core lint-proto breaking` SHALL pass against the base commit and a wire-breaking edit SHALL fail it. `make -C platform-core proto` SHALL produce Go, Python, JVM and TS outputs that expose every new RPC and message. `promotion.proto`, `order.proto` and `listing.proto` SHALL be byte-identical in each consumer `proto/` path listed in the proposal, and a missing or drifted copy SHALL fail the copies check.

#### Scenario: Seller centre contract: Change is backward compatible
- **WHEN** `make -C platform-core lint-proto breaking` runs on the change before the slice commit
- **THEN** both targets exit 0
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `buf breaking vs $BASE_REF`

#### Scenario: Seller centre contract: Wire-breaking edit is rejected
- **WHEN** one new field is renumbered in a throwaway copy of the committed change and `make -C platform-core breaking` runs
- **THEN** it exits non-zero, and the mutation is discarded afterwards
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `renum`

#### Scenario: Seller centre contract: New contract in every generated language
- **WHEN** `make -C platform-core proto` completes
- **THEN** the generated Go, Python, JVM and TS outputs each contain `ListMyCampaigns`, `CancelMyCampaign`, `ListMyShopVouchers`, `DeactivateMyShopVoucher`, `AddBundleToCart` and `OrderStatusCount`
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `gen_has`

#### Scenario: Seller centre contract: Copies match the source
- **WHEN** `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` runs after the copies are written
- **THEN** `cmp` reports each of the 36 consumer copies identical to the source and the script exits 0
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `check_copies`

#### Scenario: Seller centre contract: Drifted or missing copy is rejected
- **WHEN** the copies script runs with one consumer copy edited by one byte and then with another copy deleted, in a scratch copy of the tree
- **THEN** each run exits non-zero and names the drifted or missing path
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `drifted and missing copy rejected and named`

#### Scenario: Seller centre contract: Consumer copies lint
- **WHEN** `buf lint` runs in the `proto/` directory of each consumer repo holding a changed copy
- **THEN** every run exits 0
- **VERIFIED BY**: `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` `buf lint $r`
