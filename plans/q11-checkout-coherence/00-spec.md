# 00-spec — Multi-shop checkout coherence (payment, shipping, vouchers, selection, ETA, address)

initiative: q11-checkout-coherence

Source: backlog Q11, Q12 (ETA + address parts), Q18 (`plans/pm-review-2026-10/BACKLOG.md`); evidence
`plans/pm-review-2026-10/buyer-journey.md` #2, #3, #8, #10.
Base: `origin/main` after q10-order-lifecycle merges (Q1, Q3, Q4, Q5 and q10 merged); build starts only then,
because both initiatives edit team-order `internal/service/order.go`, `internal/handler/order.go` and
team-frontend `(checkout)/checkout/pay/[id]/page.tsx`. Builds on Q5: team-order is the price authority, the
frontend shows server figures only, `CalculateShippingFee` is reachable through team-gateway, multi-shop
parity was deferred to here.

## Goal
A buyer whose cart spans several shops sees, per shop group, the subtotal, shipping fee, shop-voucher
discount, share of the platform voucher, total and an estimated delivery date, and these are exactly the
figures of the orders CreateOrder places. Every online-pay order of a checkout has a payable transaction.
The buyer can tick which lines (or whole shops) to check out, and "Mua ngay" checks out only that line.
An order is never placed at an address the buyer did not pick and see at confirm.

## User stories
1. **Per-shop totals match the charge.**
   - Given a selection with items from shop A (300.000đ) and shop B (300.000đ) and an address in Đà Nẵng.
   - When the buyer opens checkout.
   - Then two groups are shown, each with subtotal 300.000đ and shipping 35.000đ (not "free"), and the
     summary total equals the sum of the two placed orders' `total_amount`.
   - Edge cases:
     - A group whose own subtotal is ≥ 500.000đ ships free; the threshold is per group, never on the whole selection.
     - The figures come from `PreviewCheckout`; the frontend adds nothing up beyond rendering them.
2. **Every online-pay order can be paid.**
   - Given a two-shop checkout with an online payment method.
   - When the order is placed.
   - Then each order has its own payment (one payment per order); the buyer lands on the pay page of the first
     and, after it, is offered the next unpaid order of the same checkout (`checkout_id`) until none is left.
   - Edge cases:
     - "Thanh toán ngay" on `/account/orders` for an order without a transaction creates it and shows the pay
       page (no 404).
     - A COD checkout creates no transaction.
     - The pay page creates a missing transaction with the existing `CreatePayment`; no team-payment change.
3. **Vouchers are scoped.**
   - Given shop A's voucher, shop B's voucher and one platform voucher.
   - When the buyer applies them at checkout.
   - Then each shop voucher discounts only its shop's group; the platform voucher is computed once on the
     selection subtotal and split across groups proportionally to each group's `items_subtotal` (integer
     division; the remainder goes to the largest group, ties → lowest seller id); each order stores its own share.
   - Edge cases:
     - A shop voucher applied to another shop's group is refused with the promotion reason; a shop code sent as
       the platform code (or the reverse) is refused.
     - Stacking: one group may carry its shop voucher and its platform-voucher share at the same time; the group
       total is clamped at 0.
     - The platform voucher's `min_spend` is checked on the whole selection subtotal, never per group.
     - An invalid voucher in the preview shows its reason and the totals without it; at CreateOrder it aborts
       the checkout as today (`ErrVoucherRejected`), nothing stays held.
4. **Cart selection and true "Mua ngay".**
   - Given a cart with 3 lines across 2 shops.
   - When the buyer ticks one line (or a shop's select-all) and presses "Mua hàng".
   - Then checkout opens with `?items=<cartItemId,…>`, shows and places only those lines, and the unticked lines
     stay in the cart.
   - Edge cases:
     - "Mua ngay" on the PDP adds the variant and opens checkout with only that cart line; if the line already
       existed its merged quantity is checked out (AddToCart merges by listing + variant).
     - `?items=` naming no current cart line sends the buyer back to `/cart`; ids not in the cart are ignored.
     - Nothing ticked disables "Mua hàng".
5. **Delivery promise.**
   - Given a signed-in buyer with a default address.
   - Then the PDP shows "Nhận hàng dự kiến dd/mm" and every checkout group shows its own date, from the
     `estimated_delivery_at` of `CalculateShippingFee` / `PreviewCheckout`. Mock table: city Hồ Chí Minh or Hà Nội
     → today + 2 calendar days, any other city → today + 4 calendar days, "today" in Asia/Ho_Chi_Minh.
   - Edge cases: an anonymous visitor or a buyer without an address sees no date (never an invented one).
6. **Address integrity.**
   - Given a buyer at confirm with address X shown.
   - When CreateOrder runs and `address_id` is not one of the buyer's addresses, or the buyer has no address,
     then it fails `FAILED_PRECONDITION` ("address not found"); when identity is unreachable it fails
     `UNAVAILABLE`; nothing is reserved in either case.
   - When the address X was edited after confirm (`confirmed_address` differs from identity's copy), CreateOrder
     fails `FAILED_PRECONDITION` ("address changed") and the checkout page refreshes to show the new address.
   - The order stores the address it was placed with (snapshot, as today); shipping is never quoted for city "".

## Non-goals
- A real payment rail, IPN or reconciliation; mock payments only.
- Unpaid-order deadline and auto-cancel (owned by q10-order-lifecycle, merged before this builds); reminders.
  "Thanh toán lại" after a FAILED transaction needs no team-payment change either way: `CreatePayment` returns
  the existing transaction and `ProcessMockPayment` re-settles PENDING or FAILED.
- Notifications for placed/paid orders (Q9); grouping orders by checkout in the order list.
- Shipping-method choice (standard/express), carrier integration, per-item weight pricing.
- Server-persisted cart selection; selection lives in the URL only.
- Fixing voucher quota counting holds (known team-promotion gap) or making `ListVouchers` private.

## Architecture decisions
- Payment model: one payment per order through the existing `PaymentService.CreatePayment` (idempotent per
  order: returns the existing transaction); the pay page creates a missing transaction and offers the next
  unpaid order of the same checkout — Why: no team-payment change, settlement/refund/holdback stay per order;
  a checkout-session payment would need a new aggregate in team-payment.
- team-order is the checkout quote authority: a new read-only `OrderService.PreviewCheckout` runs the same
  grouping, shipping, voucher and proration code as `CreateOrder` and reserves nothing — Why: preview =
  charge by construction; Q5 rule "the frontend shows server figures only".
- Seller groups stay one order each (`groupBySeller`); each group's shipping is `CalculateShippingFee(city,
  group.items_subtotal)` as today — Why: the server already charges per group; only the display was wrong.
- Shop vouchers: one `ValidateAndReserve` per voucher on its group (reservation id = that order id, as today).
  Platform voucher: one `ValidateAndReserve` on the selection subtotal with empty `seller_id`
  (reservation id = `platform:<checkout_id>`), discount split across groups by team-order — Why: quota is
  consumed once; promotion keeps its single-subtotal discount math.
- Platform voucher lifecycle: counted as used (committed) once the first order of its checkout is paid;
  released only if every order of that checkout is cancelled before any is paid. When one order is cancelled
  the others keep their share and nothing is re-priced. The rule applies to buyer cancel and to q10's
  unpaid-order expiry sweeper (system cancel), keyed by `checkout_id` — Why: never re-price a placed order.
- `ValidateAndReserveResponse` echoes the voucher's `scope` and `seller_id` so team-order can route
  `voucher_code` (legacy single field) and refuse a code sent in the wrong slot — Why: no extra GetVoucher round-trip.
- Platform-voucher `min_spend` is checked on the whole selection subtotal (one ValidateAndReserve on it).
- Shop voucher and platform voucher stack on one group; proration of the platform discount: integer division by
  `items_subtotal` share, remainder to the largest group (ties → lowest seller id).
- `Order.checkout_id` (= saga id) links the orders of one checkout — Why: the pay page finds "next unpaid
  order", the platform voucher finds its siblings, Q9/Q20 can group later.
- Address resolution is fail closed in the `CreateOrder` handler: empty `address_id` → default address; unknown
  id or no address → `FAILED_PRECONDITION`; identity error or no client → `UNAVAILABLE` (retryable, mirrors
  Q5's promotion outage; the backlog's "FailedPrecondition" wording is kept for the missing-address case).
- ETA is a mock rate table in team-order next to the fee rule (HCM / Hà Nội +2, elsewhere +4 calendar days); `estimated_delivery_at` is computed by the
  server (now + days), the frontend only formats dd/mm in Asia/Ho_Chi_Minh — Why: no frontend copy of the rule.
- Cart selection is URL state (`?items=`), carried by `checkoutHref`; "Mua ngay" reads the cart line id from
  the AddToCart response (listing_id + variant_id) — Why: no contract change, matches existing `item_ids`.
- No new dependencies.

## Contracts
```proto
// platform/order/v1/order.proto — additive
service OrderService {
  // Read-only quote of a checkout for the calling user: the orders CreateOrder would place with the
  // same request, with every figure. Reserves nothing; a rejected voucher is reported, not raised.
  // FAILED_PRECONDITION when item_ids match no cart line ("cart is empty").
  rpc PreviewCheckout(PreviewCheckoutRequest) returns (PreviewCheckoutResponse);
}

message ShopVoucher {
  string seller_id = 1;
  string code = 2;
}

message CreateOrderRequest {
  // 1-4 unchanged. voucher_code (4) stays: routed by the voucher's scope (SHOP → its seller's group,
  // PLATFORM → split across groups); refused when shop_vouchers/platform_voucher_code also name it.
  repeated ShopVoucher shop_vouchers = 5;            // at most one per seller_id
  string platform_voucher_code = 6;
  // The address shown at confirm; when set and different from identity's copy of address_id
  // (any of recipient_name, phone, street, ward, district, city) → FAILED_PRECONDITION "address changed".
  platform.identity.v1.Address confirmed_address = 7;
}

message PreviewCheckoutRequest {
  string address_id = 1;                             // empty = default address
  repeated string item_ids = 2;                      // empty = whole cart
  repeated ShopVoucher shop_vouchers = 3;
  string platform_voucher_code = 4;
}

message CheckoutGroupQuote {
  string seller_id = 1;
  repeated string item_ids = 2;                      // cart item ids in this group
  int64 items_subtotal = 3;
  int64 shipping_fee = 4;
  bool is_free_shipping = 5;
  string shop_voucher_code = 6;                      // applied (valid) shop voucher, else empty
  int64 shop_discount_amount = 7;
  int64 platform_discount_amount = 8;                // this group's share of the platform voucher
  int64 total_amount = 9;                            // items_subtotal + shipping_fee - both discounts, >= 0
  google.protobuf.Timestamp estimated_delivery_at = 10; // unset when shipping_quoted = false
}

message VoucherQuote {
  string code = 1;
  platform.promotion.v1.VoucherScope scope = 2;
  string seller_id = 3;                              // group it was applied to; empty for platform
  bool valid = 4;
  string reason = 5;                                 // promotion's reason when valid = false
  int64 discount_amount = 6;                         // total discount of this voucher
}

message PreviewCheckoutResponse {
  repeated CheckoutGroupQuote groups = 1;            // sorted by seller_id, one per order CreateOrder places
  repeated VoucherQuote vouchers = 2;
  int64 items_subtotal = 3;                          // sums over groups
  int64 shipping_fee = 4;
  int64 discount_amount = 5;
  int64 total_amount = 6;
  bool shipping_quoted = 7;                          // false when the buyer has no resolvable address
  platform.identity.v1.Address shipping_address = 8; // the address the orders would use; sent back as confirmed_address
}

message CalculateShippingFeeResponse {
  // 1-3 unchanged
  int32 estimated_delivery_days = 4;
  google.protobuf.Timestamp estimated_delivery_at = 5; // now + estimated_delivery_days
}

message Order {
  // 1-17 unchanged; discount_amount (15) stays the total discount (shop + platform share),
  // voucher_code (16) is the shop voucher.
  // 18-24 are owned by q10-order-lifecycle (merges first).
  string checkout_id = 25;                           // shared by every order of one checkout
  string platform_voucher_code = 26;
  int64 platform_discount_amount = 27;               // part of discount_amount
}

// platform/promotion/v1/promotion.proto — additive
message ValidateAndReserveResponse {
  // 1-4 unchanged
  VoucherScope scope = 5;
  string seller_id = 6;                              // the shop of a SHOP voucher
}
```
Exact names and the platform reservation-id shape are fixed by the contract slice's spec. Field numbers of
`CreateOrderRequest` (5-7), `CalculateShippingFeeResponse` (4-5) and `ValidateAndReserveResponse` (5-6) are
provisional: the contract slice re-verifies the next free numbers against platform-core at build time, since
q10-order-lifecycle may also add fields to `CreateOrderRequest` / `Order`.
Existing contracts consumed unchanged: `OrderService.CreateOrder` (`item_ids`), `OrderService.GetOrder`,
`OrderService.ListBuyerOrders`, `CartService.GetCart`, `CartService.AddToCart`, `VoucherService.CommitReservation`,
`VoucherService.ReleaseReservation`, `VoucherService.ListVouchers`, `PaymentService.CreatePayment`,
`PaymentService.GetPayment`, `PaymentService.ProcessMockPayment`, `AddressService.ListAddresses`, `order.events/OrderPaid`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| Checkout quote, scoped vouchers, ETA and address fields in the contract | q11-checkout-contract | platform-core | – | provides: OrderService.PreviewCheckout, CreateOrderRequest.shop_vouchers/platform_voucher_code/confirmed_address, CalculateShippingFeeResponse.estimated_delivery_*, Order.checkout_id/platform_voucher_code/platform_discount_amount, ValidateAndReserveResponse.scope/seller_id | platform-core (+ proto copies) |
| Voucher validation reports its scope | promotion-voucher-scope | team-promotion | q11-checkout-contract | uses: VoucherService.ValidateAndReserve (fills scope, seller_id) | team-promotion, platform-e2e |
| Delivery date quoted; orders only at a confirmed address | order-address-eta | team-order | q11-checkout-contract | uses: CalculateShippingFeeResponse.estimated_delivery_*, AddressService.ListAddresses, CreateOrderRequest.confirmed_address | team-order, platform-e2e |
| Multi-shop checkout quoted and charged per shop, vouchers scoped | order-multishop-checkout | team-order | promotion-voucher-scope, order-address-eta | uses: OrderService.PreviewCheckout, Order.checkout_id/platform_*, VoucherService.ValidateAndReserve (scope), CommitReservation, ReleaseReservation; also applies C4 release on q10's system cancel | team-order, team-gateway (forwarder), platform-e2e |
| Checkout shows per-shop groups, ETA, vouchers; every order payable | frontend-checkout-coherence | team-frontend | order-multishop-checkout | uses: OrderService.PreviewCheckout, CreateOrder, CalculateShippingFee (PDP ETA), PaymentService.CreatePayment, GetPayment, ListBuyerOrders | team-frontend, platform-e2e |
| Cart selection and true "Mua ngay" | frontend-cart-selection | team-frontend | frontend-checkout-coherence | uses: CartService.GetCart, AddToCart, CreateOrder.item_ids, PreviewCheckout.item_ids | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-order/internal/service/order.go` (CalculateShippingFee, CreateOrdersFromCart, runCheckout) — order-address-eta, order-multishop-checkout.
- `team-order/internal/handler/order.go` (CreateOrder address resolution, CalculateShippingFee, new PreviewCheckout) — order-address-eta, order-multishop-checkout.
- `team-frontend/src/app/(checkout)/checkout/(summary)/page.tsx` — frontend-checkout-coherence (groups from PreviewCheckout, `?items=` passed through), frontend-cart-selection (empty-selection redirect, "Quay lại giỏ hàng" keeps the selection).
- `team-frontend/src/features/order/checkoutParams.ts`, `PlaceOrderForm.tsx` — frontend-checkout-coherence (`items`, vouchers, confirmed address), frontend-cart-selection (href from the cart).
- `platform-e2e/tests/e2e/support/` checkout and voucher helpers — any slice that adds a helper.
- `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.tsx` — frontend-checkout-coherence only here.

Cross-initiative hot files (q10-order-lifecycle edits them too; this initiative builds after q10 merges, on
main): `team-order/internal/service/order.go`, `team-order/internal/handler/order.go`,
`team-frontend/src/app/(checkout)/checkout/pay/[id]/page.tsx`, `platform/order/v1/order.proto` (`Order` 18-24).

order-multishop-checkout tasks include: apply C4 on system cancel — q10's unpaid-order expiry sweeper releases
the platform voucher by `checkout_id` only when every order of the checkout is cancelled and none was paid.

## Out of initiative
- team-payment: no change (one payment per order; CreatePayment is idempotent per order and checks PENDING +
  buyer; ProcessMockPayment re-settles PENDING or FAILED, so retry after FAILED needs nothing new).
- Gateway policy entry for `PreviewCheckout` (user, read) is a task of order-multishop-checkout, mirroring the
  `CalculateShippingFee` forwarder in `team-gateway/internal/edge/order.go`.
- `platform-core/docs/ROADMAP.md` promotion note ("platform campaigns at checkout, first seller group only") is
  updated by order-multishop-checkout (doc, not a requirement).

## NFR and data
- NFR: `PreviewCheckout` p95 ≤ 300 ms on the local stack for ≤ 5 groups: one GetCart, one ListAddresses, one
  ValidateAndReserve per applied voucher (preview namespace, never committed); no stock or flash call. CreateOrder
  adds at most one ValidateAndReserve per extra voucher. An identity outage never places an order.
- Money: int64 VND; proration uses integer division with the remainder assigned deterministically; for every
  checkout, Σ group totals = response total = Σ placed `total_amount`.
- Personal data: no new collection. `confirmed_address` / `shipping_address` are the buyer's own address
  (already stored on the order as the delivery snapshot, contract-performance basis); returned only to the
  owning user, never logged (log address ids only).
- Risk routing expected: proto (contract slice), payment (order totals, voucher money; human review), pii (address
  compare), auth (new user RPC through the gateway).

## Conventions
- Build order: after q10-order-lifecycle merges; base branch = main with q10 merged. Rebase, never hand-merge,
  the shared team-order and pay-page files.
- Mirror the checkout saga: `team-order/internal/service/order.go` `runCheckout`, `groupBySeller`, `selectItems`;
  voucher hold in `internal/service/redemption.go`; commit on settle in `internal/consumer/payment.go`;
  compensation in `saga.go` `failAndCompensate`; Q5 price pre-check `precheckPrices` (refuse before reserving).
- PreviewCheckout and runCheckout share one pure quote function (groups, fees, discounts, proration); unit-test it
  table-driven in `team-order/internal/service/`.
- Voucher tests: `team-promotion/internal/service/voucher_test.go`, handler preview namespace in
  `internal/handler/voucher.go` (`PreviewReservationPrefix`).
- Frontend: server figures only (Q5); fabricated-text guard `team-frontend/src/features/listing/fabricatedText.ts`;
  PDP shipping in `features/listing/ShippingInfo.tsx`; pay page `app/(checkout)/checkout/pay/[id]/page.tsx`
  calls CreatePayment when GetPayment is NotFound and the order is PENDING online-pay.
- E2E: extend the two-seller checkout features in `platform-e2e/tests/e2e/features/` (order
  `oic-checkout-a-two-seller-checkout-places-one-order-per-seller`); seed shop and platform vouchers through
  `VoucherService.CreateVoucher`.

## Locked
2026-10-16 — Confirmed by Bùi Đăng Phúc (recommended defaults accepted). One payment per order, platform voucher split by items_subtotal (remainder to largest group) and stacks with shop vouchers, used on first paid order, min spend on whole selection, mock ETA +2/+4 days, full scope; `Order` fields 25-27; builds after q10-order-lifecycle.

## Clarifications — spec-clarify round 1 (2026-10-16, Bùi Đăng Phúc: "approve hết", all recommended answers accepted)
Money / risk
- M1 ProcessMockPayment re-reads the order (team-order GetOrder) before settling; a CANCELLED order refuses settle (closes the CreatePayment-vs-cancel race). Owner: payment-unpaid-cancel.
- M2 An APPROVED return on a COD order (no PaidAt) does not block auto-complete; COD refunds are a follow-up. Owner: order-receipt-completion.
- M3 No auto-refund of an APPROVED return the seller never refunds; follow-up.
- M4 ReserveStock / checkout on a HELD listing is refused FAILED_PRECONDITION. Owner: domain-seller-enforcement-hold (team-domain reservation path).
- M5 Legacy CreateOrder.voucher_code is treated as the platform-voucher slot only; no SHOP auto-routing probe. Owner: order-multishop-checkout.
Trust (q6)
- A8 Every seller content edit of a PUBLISHED listing is re-screened; a flagged edit moves it to PENDING_REVIEW. Status omitted must be distinguishable from DRAFT (fix domainStatus UNSPECIFIED handling).
- A9 Admin REJECT/BAN allowed from PENDING_REVIEW and PUBLISHED; no reversal of REJECTED/BANNED (APPROVE refused).
- A10 Seller DeleteListing on BANNED/HELD → FAILED_PRECONDITION.
- A11 screen_flags are admin-only; the owner sees reason_code + note only.
- A12 An admin's own listing follows the seller path (screen runs), still audited.
- A13 ListingChanged events carry no moderation note and no screen flags.
- A14 SetUserStatus RESTRICTED/SUSPENDED/BANNED (including re-setting the same status) revokes all sessions; ACTIVE revokes none. Contract and identity slice must say the same.
- A15 Expired suspension is lifted only at the user's next login (listings stay HELD until then); a lift sweeper is a follow-up.
- A16 Login refusal is one generic message ("tài khoản bị khóa"); no suspended/banned distinction or end time.
- A17 Dispute state machine allows OPEN→INVESTIGATING and OPEN→REJECTED (spam); INVESTIGATING→{RESOLVED,REJECTED}. A ruling with any status other than RESOLVED is INVALID_ARGUMENT (contract comment changes to match).
- A18 Re-escalation of a return after its dispute closed is allowed while can_escalate is true. ListOrderDisputes is allowed to the order's buyer and seller (and admin).
- A19 Max 20 OPEN reports per reporter (RESOURCE_EXHAUSTED); ResolveReport note required, 1–500 chars.
- A20 A restricted seller visiting seller pages sees a "Tài khoản bị hạn chế" notice instead of a /login redirect loop. Owner: frontend-user-enforcement.
Order lifecycle (q10)
- B7 Pre-deploy orders: unpaid orders get no payment_due_at (never auto-expired); backfill SHIPPED auto_complete_at = migration+7d, COMPLETED return_window_ends_at = updated_at+15d, PENDING returns auto_approve_at = migration+3d (fixed values, stated in the proposal).
- B8 ConfirmReceipt is allowed while the buyer's return is PENDING or APPROVED.
- B9 A buyer sending a system-only cancel reason gets PERMISSION_DENIED.
- B10 Buyer notifications: ReturnRefunded yes; OrderCancelled only when reason is PAYMENT_EXPIRED; OrderCompleted always.
Checkout (q11)
- C8 VoucherQuote.scope uses a local enum in order.proto (no import of promotion.proto, no extra proto copies).
- C9 A buyer with addresses but no default and an empty address_id → FAILED_PRECONDITION "address not found" (no first-address fallback).
- C10 Cart default selection = all ticked; on a partial selection /cart hides the voucher row and says "Giảm giá được tính ở bước thanh toán".
Cross-slice fixes (orchestrator)
- q6 engagement proto: q10 no longer touches engagement.proto — the "coordinate field numbers with q10" note is moot; q6-trust-contract owns them.
- q10 contract test must NOT assert "no Order field >= 25" (q11 uses 25–27).
- q6 ownership: duplicate scenario titles between engagement-listing-reports / frontend-listing-moderation / frontend-user-enforcement must be made unique (prefix each with its slice's capability, e.g. "Moderation UI: …", "Users UI: …").
- Every frontend-called RPC must have a gateway forwarder in a named slice (PreviewCheckout → order-multishop-checkout; ConfirmReceipt → order-receipt-completion; AccountAdminService → identity-user-enforcement; ListOrderDisputes/ListDisputes → engagement-dispute-escalation).
- The mock screen default must ALLOW clean text so existing e2e seeds that publish listings still auto-publish.
NEEDS_HUMAN before release: DPO (enforcement record basis, 2-year report retention, Langfuse traces when a real LLM is on, dispute audit metadata); legal sign-off on the prohibited-term list; Kafka ACL on identity.events.
