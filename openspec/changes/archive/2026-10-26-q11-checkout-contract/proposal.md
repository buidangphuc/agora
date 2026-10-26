## Why
The q11-checkout-coherence initiative makes a multi-shop checkout show, and charge, the same per-shop figures: subtotal, shipping, shop-voucher discount, platform-voucher share, total and an estimated delivery date; scopes vouchers; and refuses to place an order at an address the buyer did not confirm. None of this is expressible in the current contract: `OrderService` has no quote RPC, `CreateOrderRequest` carries a single `voucher_code`, `Order` has no link between the orders of one checkout, `CalculateShippingFeeResponse` has no delivery date, and `ValidateAndReserveResponse` does not say whether a voucher is a SHOP or PLATFORM voucher. This slice adds those fields and the `PreviewCheckout` RPC, purely additively, and refreshes the existing proto copies. Behaviour lives in `promotion-voucher-scope`, `order-address-eta`, `order-multishop-checkout` (team-order, team-gateway) and the two frontend slices.

initiative: q11-checkout-coherence

## What changes
All changes are additive. Field numbers were verified against `platform-core/packages/proto` on 2026-10-16 (base: main before q10-order-lifecycle merges; `Order` ends at 17 there, `CreateOrderRequest` at 4, `CalculateShippingFeeResponse` at 3, `ValidateAndReserveResponse` at 4).

`platform/order/v1/order.proto`:
- No new import: `order.proto` keeps importing only `timestamp`, `identity` and `payment`. A new local enum `CheckoutVoucherScope` (local enum in `order.proto`: `CHECKOUT_VOUCHER_SCOPE_UNSPECIFIED = 0`, `CHECKOUT_VOUCHER_SCOPE_SHOP = 1`, `CHECKOUT_VOUCHER_SCOPE_PLATFORM = 2`, the same numbers as `platform.promotion.v1.VoucherScope`) types `VoucherQuote.scope`; the two enums are kept in step by the compat test.
- `OrderService` gains `rpc PreviewCheckout(PreviewCheckoutRequest) returns (PreviewCheckoutResponse)`: read-only quote of the orders `CreateOrder` would place for the calling user; reserves nothing; a rejected voucher is reported in the response, not raised; `FAILED_PRECONDITION` ("cart is empty") when `item_ids` match no cart line. User-callable through the gateway (policy: user, read; added in order-multishop-checkout).
- New messages: `ShopVoucher {seller_id=1, code=2}`; `PreviewCheckoutRequest {address_id=1, item_ids=2 (repeated), shop_vouchers=3 (repeated ShopVoucher), platform_voucher_code=4}`; `CheckoutGroupQuote {seller_id=1, item_ids=2 (repeated), items_subtotal=3, shipping_fee=4, is_free_shipping=5, shop_voucher_code=6, shop_discount_amount=7, platform_discount_amount=8, total_amount=9, estimated_delivery_at=10 (Timestamp)}`; `VoucherQuote {code=1, scope=2 (CheckoutVoucherScope), seller_id=3, valid=4, reason=5, discount_amount=6}`; `PreviewCheckoutResponse {groups=1 (repeated), vouchers=2 (repeated), items_subtotal=3, shipping_fee=4, discount_amount=5, total_amount=6, shipping_quoted=7 (bool), shipping_address=8 (platform.identity.v1.Address)}`. Money fields are int64 VND.
- `CreateOrderRequest` gains `repeated ShopVoucher shop_vouchers = 5`, `string platform_voucher_code = 6`, `platform.identity.v1.Address confirmed_address = 7`. Field 4 `voucher_code` stays and is documented as the platform-voucher slot only (same as `platform_voucher_code`, no SHOP routing).
- `CalculateShippingFeeResponse` gains `int32 estimated_delivery_days = 4` and `google.protobuf.Timestamp estimated_delivery_at = 5`.
- `Order` gains `string checkout_id = 25`, `string platform_voucher_code = 26`, `int64 platform_discount_amount = 27`. Numbers 18-24 are reserved for q10-order-lifecycle (merges first) and are NOT used or declared here; `discount_amount` (15) stays the total discount (shop + platform share) and `voucher_code` (16) the shop voucher.

`platform/promotion/v1/promotion.proto`: `ValidateAndReserveResponse` gains `VoucherScope scope = 5` and `string seller_id = 6` (the shop of a SHOP voucher; empty for PLATFORM).

Documented semantics (proto comments, pinned by the compat test):
- `PreviewCheckout` figures equal what `CreateOrder` places for the same request: one group per seller, sorted by `seller_id`; `groups[].total_amount = items_subtotal + shipping_fee - shop_discount_amount - platform_discount_amount`, clamped at 0; response totals are the sums over groups; free shipping is decided per group on its own `items_subtotal`.
- Voucher routing: legacy `voucher_code` is the platform-voucher slot only; a shop voucher discounts only its seller's group; the platform voucher is computed once on the selection subtotal (its `min_spend` is checked on the whole selection) and split across groups proportionally to `items_subtotal` with integer division, the remainder to the largest group (ties: lowest `seller_id`). A shop voucher may stack with a platform share on one group. A code sent in the wrong slot (a SHOP code in `voucher_code` / `platform_voucher_code`, a PLATFORM code or another seller's code in `shop_vouchers`) makes `CreateOrder` fail `FAILED_PRECONDITION` with the promotion reason. A second `shop_vouchers` entry for one `seller_id`, or `voucher_code` set together with `platform_voucher_code` or repeating a `shop_vouchers` code, fails `INVALID_ARGUMENT`. Nothing stays held in either case.
- Platform reservation id on `ValidateAndReserve`: `platform:<checkout_id>`; shop vouchers keep the order id.
- `CreateOrder` fails closed on address: empty `address_id` means the default address; unknown `address_id`, no address, or addresses but none marked default with an empty `address_id` (no first-address fallback) gives `FAILED_PRECONDITION` ("address not found"); a `confirmed_address` differing from identity's copy in any of recipient_name, phone, street, ward, district, city gives `FAILED_PRECONDITION` ("address changed"); identity unreachable gives `UNAVAILABLE`; nothing is reserved in any of these cases.
- `estimated_delivery_at` is computed by the server (now + `estimated_delivery_days`); unset when `shipping_quoted` is false.
- `Order.checkout_id` is shared by every order of one checkout (equals the saga id).

Copies (byte-identical, never edited by consumers): `order.proto` already exists in team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-gateway, team-notification, team-order, team-payment, team-search (all identical to the source today) and is re-copied to each. `promotion.proto` is re-copied only where it already exists: team-analytics, team-frontend, team-gateway, team-order and team-promotion. No repo gains a new proto file. Consumers regenerate in their own slices.

A committed Go test `platform-core/packages/proto/platform/order/v1/checkout_compat_test.go` (same approach as `promotion/v1/flashstock_compat_test.go`) pins RPCs, messages, field numbers, comment text and wire round-trips.

Out of scope: any behaviour (grouping, proration, voucher routing, ETA table, address check, gateway route), changes to existing fields or RPCs, `Order` 18-24, team-payment.

## NFR
- Additive only: `buf breaking` (FILE) against the committed base passes; a consumer unaware of the new fields keeps working and old bytes of existing messages are unchanged.
- No new service, port or dependency; contract-only, no runtime latency impact. The `PreviewCheckout` latency target (p95 <= 300 ms on the local stack for <= 5 groups) is recorded in the RPC comment and verified in order-multishop-checkout.
- Compat: a `CreateOrderRequest` written by a new caller parses in the old schema (fields 5-7 skipped); an old-schema `Order` read by a new reader leaves 25-27 at defaults.
- Money is int64 VND everywhere; no float.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `CreateOrderRequest.confirmed_address`, `PreviewCheckoutResponse.shipping_address` (recipient_name, phone, street, ward, district, city) | personal data (the buyer's own address) | detect an address edited after confirm; show the address a quote used | contract performance (already stored on the order as the delivery snapshot; `shipping_address` is returned only to the owning buyer, never stored, never logged) | no new storage: the order snapshot is retained as today; the preview response is not stored |
| `address_id` | personal-data reference | select the buyer's address | contract performance | unchanged |
| `shop_vouchers[].code`, `platform_voucher_code`, `VoucherQuote.*` | business data | scope vouchers and report their outcome | contract | lives with the reservation (team-promotion) |
| `checkout_id`, `Order.platform_*`, amounts, `estimated_delivery_at` | business data | link and price the orders of one checkout | contract | same as the order record |
| `ValidateAndReserveResponse.scope`, `seller_id` | business data (public voucher attributes) | route a voucher | contract | none |

No new personal data is collected. Address fields are returned only to the owning buyer and must not be logged (log address ids only; documented in the RPC comments).

## Threat model
- Assets: order totals and voucher money (a quote must equal the charge); the buyer's delivery address; voucher quota.
- Abuse cases: applying shop A's voucher to shop B's group, or a shop code in the platform slot (`FAILED_PRECONDITION` with the promotion reason, owned by order-multishop-checkout); a caller passing another user's `address_id` or a forged `confirmed_address` (CreateOrder verifies against identity's copy for the authenticated buyer; a mismatch is refused); `PreviewCheckout` used to probe vouchers or burn quota (reserves nothing: preview namespace, never committed; the existing `ValidateAndReserve` limits apply); `PreviewCheckout` reading another buyer's cart or addresses (identity comes from the authenticated principal only, no buyer id in the request); a consumer copy drifting from the source (the `cmp` check).
- Authz rule: `PreviewCheckout` and the new `CreateOrder` fields are for the authenticated buyer (user principal, own cart and own addresses only); no seller or service token path. The contract documents this; enforcement and tests belong to order-multishop-checkout and order-address-eta.
