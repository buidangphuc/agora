# 00-spec — Charge the advertised price (flash sale, shipping, voucher parity)

initiative: q5-advertised-price

Source: backlog Q5 (`plans/pm-review-2026-10/BACKLOG.md`), evidence `plans/pm-review-2026-10/buyer-journey.md` #1.
Base: stacked on `asdlc/q4-loop-instrumentation` (Q2 → Q3 → Q4 → Q5 chain; Q3 touched team-order audit, Q2/Q4 touched the PDP).

## Goal
The buyer is charged exactly the price the shop showed them. A flash-sale price shown on the PDP is the unit price in the cart and on the order, and flash-sale stock is capped by the campaign's `stock_cap`. When the price at checkout no longer matches the cart (the sale ended, was switched off or sold out), checkout is refused, the cart line is re-priced, and the buyer sees the new price before placing the order again. The PDP and checkout show shipping figures from team-order's `CalculateShippingFee`, not a copy of the rule or invented text; the voucher preview is computed on the same subtotal the server charges.

## User stories
1. **Flash price is charged.**
   - Given a listing (or variant) with an active flash-sale campaign whose `sale_price` is below the regular price and whose remaining stock covers the quantity.
   - When a buyer adds it to the cart and checks out.
   - Then the cart line's unit price is `sale_price`, the placed order item's `unit_price` is `sale_price`, the order total is built from it, and the campaign's `stock_sold` grows by the quantity when the order is placed (and shrinks back if the order is cancelled).
   - Edge cases:
     - A campaign for one variant prices only that variant; a campaign with empty `variant_id` prices every variant of the listing.
     - The flash-sale kill-switch OFF means no flash price (regular price everywhere).
     - Without an active campaign the regular (variant) price is used, as today.
2. **Price change is refused, not charged.**
   - Given a cart line priced at a flash price.
   - When at checkout the campaign has ended, is switched off, or has fewer remaining units than the line's quantity.
   - Then CreateOrder fails with `FAILED_PRECONDITION` and the reason "price changed", reserves nothing (no stock, voucher or flash hold stays), the cart line's unit price is updated to the current price, and the checkout page shows the buyer a message and the new total.
   - Edge cases:
     - A line whose current price is lower than the cart price (a sale started after add) is also a price change: refused and re-priced the same way.
     - A whole line is either fully at the flash price or fully at the regular price; it is never split.
     - Placing the order again after the re-price succeeds at the new price.
3. **Flash stock is capped.**
   - Given a campaign with `stock_cap = N`.
   - When concurrent checkouts reserve more than N units in total.
   - Then at most N units are sold at the flash price; the rest are refused as in story 2. A failed or cancelled checkout returns its held units; `GetFlashSaleStock.remaining` reflects held + sold units.
4. **Shipping copy is real.**
   - Given any visitor on a PDP.
   - Then the shipping row shows the free-shipping threshold (500.000đ) and, for a signed-in buyer with a default address, the fee `CalculateShippingFee` returns for that city below the threshold; an anonymous visitor (or no address) sees the threshold and the 20.000đ–35.000đ range. The texts "Miễn phí vận chuyển cho đơn hàng từ 0Đ", "SPX Express (1-2 ngày)" and "Bảo hiểm thiết bị và bảo vệ người mua" are never rendered.
5. **Checkout totals match the charge.**
   - Given a single-shop cart on checkout.
   - Then the shipping fee shown comes from `CalculateShippingFee(city, subtotal)` on the server cart subtotal, the voucher preview uses that same subtotal (which already carries flash prices), and the shown total equals the placed order's `total_amount`.

## Non-goals
- Multi-shop coherence: per-shop shipping and voucher scoping, payment for every order of a split cart (Q11).
- A delivery promise / ETA on the PDP (Q12).
- Seller flash-sale UI (Q27); per-buyer purchase limits on a flash sale.
- A reconciliation job or storing the price source on the order item.
- Fixing voucher quota counting holds (separate known gap) or making `ListVouchers` private.

## Architecture decisions
- team-order is the price authority: AddToCart resolves the unit price through team-promotion `GetActiveFlashSale`, and checkout re-resolves every line before reserving anything — Why: the cart snapshot is what the buyer saw; the server must not charge a price nobody showed.
- A mismatch between cart price and current price refuses the whole checkout (`FAILED_PRECONDITION`, "price changed") and re-prices the cart line — Why: the buyer is never charged a price they did not see, and never silently loses a sale price.
- Flash stock uses a hold like team-domain stock: new FlashSaleService RPCs reserve (Phase A, idempotent on `reservation_id`, expires after 15 minutes if uncommitted), commit (Phase B, next to the team-domain CommitReservation) and release (compensation and cancel; also returns committed units) — Why: concurrent buyers must not exceed `stock_cap`, a crash never strands units, and a cancelled order frees flash stock; mirrors ADR-0008. (Changed 2026-10-16 by spec-clarify Round 1, human: was "commit on OrderPaid, no TTL".)
- The flash hold RPCs are service-only (team-order's service principal); they are not exposed through the gateway — Why: a browser must not hold or release campaign stock.
- `GetFlashSaleStock.remaining = stock_cap − stock_sold − active holds` — Why: the live meter must match what checkout can still sell.
- The frontend shows server figures only: PDP shipping and checkout shipping call `CalculateShippingFee`; `computeShippingFee` is removed — Why: a frontend copy of a server rule drifts.
- A failed `GetActiveFlashSale` at add-to-cart prices the line at the regular price (fail closed to no discount); at checkout it refuses with "price changed" only when it can prove a difference, otherwise it fails the checkout as `UNAVAILABLE` — Why: never charge a price that was not confirmed.
- No new dependencies.

## Contracts
```proto
// platform/promotion/v1/promotion.proto — append to service FlashSaleService (additive)
// Service-only (team-order). Idempotent on reservation_id: the same id returns the same
// result without holding twice. reserved=false + reason when the campaign is not active
// or remaining (cap − sold − holds) < quantity.
rpc ReserveFlashSaleStock(ReserveFlashSaleStockRequest) returns (ReserveFlashSaleStockResponse);
// Makes a hold permanent: stock_sold += quantity. Idempotent.
rpc CommitFlashSaleStock(CommitFlashSaleStockRequest) returns (CommitFlashSaleStockResponse);
// Returns a hold. Idempotent; unknown or committed id = no-op.
rpc ReleaseFlashSaleStock(ReleaseFlashSaleStockRequest) returns (ReleaseFlashSaleStockResponse);

message ReserveFlashSaleStockRequest {
  string reservation_id = 1;   // order id + listing/variant
  string campaign_id = 2;
  string listing_id = 3;
  string variant_id = 4;
  int64 quantity = 5;
  int64 expected_sale_price = 6; // refused when the campaign's price differs
}
message ReserveFlashSaleStockResponse { bool reserved = 1; string reason = 2; }
message CommitFlashSaleStockRequest { string reservation_id = 1; }
message CommitFlashSaleStockResponse { bool committed = 1; }
message ReleaseFlashSaleStockRequest { string reservation_id = 1; }
message ReleaseFlashSaleStockResponse { bool released = 1; }
```
Exact field names and the reservation-id shape are fixed by the contract slice's spec.
Existing contracts consumed unchanged: `FlashSaleService.GetActiveFlashSale`, `FlashSaleService.GetFlashSaleStock`, `OrderService.CalculateShippingFee`, `OrderService.CreateOrder`, `CartService.AddToCart`, `VoucherService.ValidateAndReserve`, `order.events/OrderPaid`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| Flash stock hold RPCs in the contract | q5-flash-stock-contract | platform-core | – | provides: FlashSaleService.ReserveFlashSaleStock, CommitFlashSaleStock, ReleaseFlashSaleStock | platform-core (+ proto copies) |
| Flash stock is held, sold and capped | promotion-flash-stock-hold | team-promotion | q5-flash-stock-contract | provides: the 3 RPCs; uses: — | team-promotion, platform-e2e |
| Cart and order charge the advertised price | order-advertised-price | team-order | promotion-flash-stock-hold | uses: FlashSaleService.GetActiveFlashSale, ReserveFlashSaleStock, CommitFlashSaleStock, ReleaseFlashSaleStock | team-order, platform-e2e |
| PDP and checkout show server prices and fees | frontend-advertised-price | team-frontend | order-advertised-price | uses: OrderService.CalculateShippingFee, OrderService.CreateOrder | team-frontend, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `platform-e2e/tests/e2e/support/` flash-sale / checkout helpers — any slice that adds a helper.
- `platform-e2e` seed for flash-sale campaigns — promotion-flash-stock-hold first.

## Out of initiative
- team-gateway: no change expected (new RPCs are service-only; `CalculateShippingFee` is already reachable). If a spec author finds a gateway route is missing, it becomes a task of the slice that needs it.
- `team-promotion/README.md` Known gaps line for `stock_sold` is removed by promotion-flash-stock-hold (doc, not a requirement).

## NFR and data
- NFR: add-to-cart adds one `GetActiveFlashSale` call (existing budget of the upstream client); checkout adds one reserve call per flash-priced line. A promotion outage never charges an unconfirmed price.
- Money: amounts are int64 VND, never floats; no rounding is introduced.
- Personal data: none new. Holds carry order id, listing/variant id and quantity only.
- Risk routing expected: proto (contract slice), payment-adjacent (order totals; human review), auth (service-only RPCs in promotion).

## Conventions
- Mirror the voucher hold: `team-order/internal/service/redemption.go`, `team-order/internal/consumer/payment.go` (commit on OrderPaid), `saga.go` `failAndCompensate`, `order.go` `CancelOrder`; promotion `ValidateAndReserve` repository and tests.
- Mirror flash-sale tests: `team-promotion/internal/service/flashsale_test.go`; PDP tests `team-frontend/src/app/(shop)/listing/[id]/page.test.tsx`; fabricated text guard `team-frontend/src/features/listing/fabricatedText.ts`.
- E2E: extend existing checkout/promotion features in `platform-e2e/tests/e2e/features/`; flash-sale campaigns are seeded through `FlashSaleService.CreateCampaign` as admin.

## Locked
2026-10-16 — Confirmed by Bùi Đăng Phúc. team-order is the price authority (flash price at add-to-cart, re-check at checkout); mismatch refuses checkout with "price changed" and re-prices the cart line; flash stock hold reserve/commit(OrderPaid)/release via service-only FlashSaleService RPCs; PDP shipping from CalculateShippingFee, SPX + insurance text removed; checkout shipping from server, voucher preview on the server subtotal; multi-shop to Q11; 4 slices; stacked on asdlc/q4-loop-instrumentation.
