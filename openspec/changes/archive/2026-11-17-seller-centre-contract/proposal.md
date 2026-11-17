## Why

The seller-centre initiative lets a seller brand the shop, run vouchers and flash sales from `/seller/marketing`, sell combos that buyers can really buy, and manage thousands of orders and listings server-side. Every backend and frontend slice needs the same wire contract first. This slice adds four seller-scoped promotion RPCs, `CartService.AddBundleToCart` with bundle fields on cart and order lines, optional paging and filter fields on `ListSellerOrders` and `ListMyListings`, and the storefront validation comments, plus the proto copies consumers carry. Behaviour lives in `promotion-seller-self-service`, `order-bundle-checkout`, `order-seller-list-paging`, `domain-my-listings-filter`, `domain-storefront-customisation` and the seller and buyer frontend slices. This slice is built first.

initiative: seller-centre

## What changes

Additive only; no existing RPC, message, field number or enum value changes.

`platform-core/packages/proto/platform/promotion/v1/promotion.proto`:
- `FlashSaleService` gains `ListMyCampaigns` and `CancelMyCampaign`; `VoucherService` gains `ListMyShopVouchers` and `DeactivateMyShopVoucher` (messages and numbers as in `design.md`). Comments: the seller comes from the principal and is never a request field; `listing.write` is required; a foreign or unknown id answers `PERMISSION_DENIED`; `CancelMyCampaign` accepts only a SCHEDULED campaign (`FAILED_PRECONDITION` otherwise); `DeactivateMyShopVoucher` is idempotent; neither is audited; the admin RPCs keep their edge policy.
- `FlashSaleCampaign.seller_id = 11` (server-owned).

`platform-core/packages/proto/platform/order/v1/order.proto` (adds `import "platform/common/v1/common.proto"`):
- `CartService.AddBundleToCart(AddBundleToCartRequest{bundle_id=1, quantity=2}) returns (AddBundleToCartResponse{cart=1})`; `FAILED_PRECONDITION` when a bundle listing is unpublished, out of stock, not owned by the bundle seller or a variant listing; `NOT_FOUND` for no bundle; `INVALID_ARGUMENT` for quantity < 1.
- `CartItem.bundle_id = 10`, `CartItem.bundle_discount = 11`, `Cart.bundle_discount_total = 4`, `OrderItem.bundle_id = 9`, `OrderItem.bundle_discount = 10`.
- `ListSellerOrdersRequest` gains `page = 2`, `query = 3`, `statuses = 4`; `ListSellerOrdersResponse` gains `page = 2` and `status_counts = 3` (new message `OrderStatusCount {status=1, count=2}`). Unset `page` keeps today's full-list behaviour.

`platform-core/packages/proto/platform/listing/v1/listing.proto`:
- `ListMyListingsRequest` gains `statuses = 2` and `query = 3` (title substring, case-insensitive, at most 100 chars).
- Comments only on `Storefront` and `UpsertStorefrontRequest`: tagline at most 120 runes and no control characters; `banner_url` an object URL from the listing-images bucket or empty; `featured_listing_ids` at most 6, unique, owned by the caller and existing, otherwise `INVALID_ARGUMENT` and nothing saved; `theme` one of `default`, `dark`, `festive`. No field change.

Copies, byte-identical to the source: `promotion.proto` in team-analytics, team-frontend-admin, team-frontend-seller, team-frontend, team-gateway, team-order, team-promotion; `order.proto` in team-analytics, team-chat, team-domain, team-engagement, team-frontend-admin, team-frontend-seller, team-frontend, team-gateway, team-notification, team-order, team-payment, team-promotion, team-referral, team-search; `listing.proto` in team-ai, team-analytics, team-chat, team-domain, team-engagement, team-frontend-admin, team-frontend-seller, team-frontend, team-gateway, team-identity, team-notification, team-order, team-payment, team-promotion, team-search. Consumers regenerate in their own slices and never edit a copy.

Committed Go tests `platform-core/packages/proto/platform/{promotion,order,listing}/v1/sellercentre_compat_test.go` (same approach as `order/v1/lifecycle_compat_test.go`) pin RPCs, messages, field numbers, comment text and wire round-trips; `platform-core/packages/proto/sellercentre_consumer_copies_test.sh` checks the copies.

## Capabilities

### New Capabilities
- `seller-centre-contract`: the wire contract for seller promotion self-service, buyable bundles, server-side paging and filters, and storefront validation notes.

### Modified Capabilities
none

## Impact

`platform-core/packages/proto/**` and the 15 consumer `proto/` copies listed above. No runtime behaviour, gateway route, migration or UI change here: gateway forwarders, edge policy for the new RPCs, and all enforcement belong to the owning slices.

## Non-goals

Any runtime behaviour; gateway routes; renumbering a field; bulk ship RPC (bulk ship is a seller-app loop over `UpdateOrderStatus`); bulk price/stock edit; storefront field changes.

## NFR

- Additive only: `make -C platform-core lint-proto breaking` passes against the base commit; an old reader skips the new fields and parses unknown enum numbers; new `Cart`, `CartItem`, `OrderItem` fields leave old bytes of existing messages unchanged.
- Contract-only: no latency or availability impact, no new service, port, image or dependency.
- Limits fixed by the contract: money is int64 VND; at most 6 featured listings; tagline at most 120 runes; `query` at most 100 chars; `quantity` at least 1; paging uses the existing `PageRequest` (server clamps `page_size`).
- Generated code for Go, Python, JVM and TS exposes every new RPC and message.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `seller_id` (campaign), `Voucher.seller_id`, `OrderItem` / `CartItem` ids | pseudonymous identifiers, existing data | scope seller lists and cart lines | contract performance, already collected | with the owning record |
| `bundle_id`, `bundle_discount`, `bundle_discount_total`, `OrderStatusCount` | business data | price a combo, show counts | contract | with the order or cart |
| `ListMyListingsRequest.query`, `ListSellerOrdersRequest.query` | seller-typed search text, may contain a buyer-visible title or order id prefix | server-side filter | legitimate interest of the seller managing own data | not stored and not logged by any slice |
| `Storefront.banner_url`, `tagline` | public seller content | shop branding | seller-provided, publicly displayed by design | until the seller changes it |

No new personal data of buyers: the contract adds ids, amounts, counts and seller-authored public text only.

## Threat model

- Assets: seller isolation (a seller cannot see or stop another seller's vouchers or campaigns), checkout price integrity (bundle discount), order list privacy.
- Abuse cases: (1) a seller passing another seller's id to read or cancel it (the contract takes the seller from the principal only and documents `PERMISSION_DENIED` identical for foreign and unknown ids, so ids cannot be probed); (2) a buyer forging `bundle_discount` on a cart line (fields are server-owned, documented as such; the order recomputes); (3) a consumer copy drifting from the source (the `cmp` check); (4) a renumbered field reinterpreting old data (`buf breaking` and the pinned descriptor tests); (5) oversized `query` or page flooding (limits documented; enforcement in owning slices).
- Authz rule: the four seller RPCs need `listing.write` and act only on the principal's own records; `AddBundleToCart` is buyer-authenticated like `AddToCart`; `ListSellerOrders` stays seller-scoped. This slice documents the rules; enforcement and tests belong to the owning slices.
