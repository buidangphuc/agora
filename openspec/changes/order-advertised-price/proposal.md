## Why

The PDP shows a flash-sale price but team-order never asks team-promotion for it: `CartService.AddToCart` (`team-order/internal/service/cart.go`) snapshots the listing or variant regular price, and `runCheckout` (`order.go`) charges the cart snapshot. A buyer is charged the regular price for an advertised sale, and flash `stock_cap` is not enforced on the order side (backlog Q5, `plans/pm-review-2026-10/buyer-journey.md` #1). The locked initiative (`plans/q5-advertised-price/00-spec.md`, 2026-10-16) makes team-order the price authority: add-to-cart resolves the flash price, checkout re-resolves every line before reserving anything and refuses with "price changed" when the price differs, and flash stock is held like team-domain stock (reserve in Phase A, commit in Phase B next to the team-domain commit, release on compensation and cancel; an uncommitted hold expires after 15 minutes in promotion). This slice is the team-order side. The three hold RPCs come from `q5-flash-stock-contract` and are implemented by `promotion-flash-stock-hold`; the PDP and checkout UI is `frontend-advertised-price`.

initiative: q5-advertised-price

## What Changes

- **team-order, upstream** (`internal/upstream/promotion.go`, `cmd/server/main.go`):
  - `PromotionClients` gains `Flash promotionv1.FlashSaleServiceClient` on the same connection and the same `UPSTREAM_PROMOTION_ADDR` (empty address still yields a nil client set, so flash pricing and holds are off and checkout is unchanged).
  - A wrapper marks `ReserveFlashSaleStock`, `CommitFlashSaleStock`, `ReleaseFlashSaleStock` as team-order's service principal with scope `promotion.reserve` (the existing scope that `servicePromotionClient` uses for the voucher hold), in a buyer request and in compensation and cancel alike. `GetActiveFlashSale` is NOT wrapped: it is a public read and keeps the forwarded principal.
- **team-order, cart** (`internal/service/cart.go`): `CartService` takes a `FlashSaleClient` (interface with `GetActiveFlashSale`) through a new variadic option `WithFlashSaleClient`. `AddToCart` resolves the unit price with one `GetActiveFlashSale(listing_id)` call: `sale_price` when the campaign is active, its `variant_id` is empty or equals the chosen variant_id, and `0 < sale_price < regular price`; otherwise the regular price, as today. A `GetActiveFlashSale` error prices at the regular price (fail closed to no discount), logged. `Reorder` already goes through `AddToCart`, so it prices the same way. The cart stores only the unit price; no price-source column is added.
- **team-order, checkout** (`internal/service/order.go`, `saga.go`, `redemption.go` pattern):
  - New pre-check in `CreateOrdersFromCart`, after the self-purchase check and before the saga/any hold (for a keyed saga it runs through `abandon`, freeing the key): for every selected line load the listing from team-domain (`GetListing`, regular price R, variant price when `> 0`) and resolve the current price P with the same rule as AddToCart (one `GetActiveFlashSale` per distinct listing). A line is flash-priced when P < R.
  - If any line has `cart unit_price != P`, the checkout returns `ErrPriceChanged`, each differing line is re-priced in the cart to P (new `CartRepository.UpdateItemPrice(ctx, userID, itemID, unitPrice)`, in memory and Postgres), and nothing is held. Higher and lower are both a change.
  - `GetActiveFlashSale` failure at checkout: if the cart price is above R, or equal to R, the line is decided without the campaign (above R is a proven change to R; equal to R is charged at R); if the cart price is below R (flash-priced, unconfirmable) the checkout returns `ErrPromotionUnavailable`, nothing held, cart untouched.
  - Phase A: for each flash-priced line `reserveGroup` calls `ReserveFlashSaleStock` after the stock reservation, with reservation id `FlashReservationID(orderID, listingID, variantID)` = `<order id>:<listing id>:<variant id>` (order id = the seller group's pre-generated order id; variant segment empty for a listing line), `campaign_id`, `listing_id`, `variant_id`, `quantity` = line quantity, `expected_sale_price` = P. `reserved=false` goes to the same refusal as a price change (team-order does not match the `reason` value; any `reserved=false` is refused, the contract's closed set is `not_active`, `price_changed`, `insufficient_stock`) (`ErrPriceChanged`, every hold of the attempt released through `failAndCompensate`, the refused line re-priced to R, other lines re-priced to their current price). A transport error goes to `ErrPromotionUnavailable` (all holds released).
  - A line is never split between flash and regular price. The order item `unit_price` and `itemsSubtotal` are the charged price; the voucher `ValidateAndReserve` receives that flash-priced subtotal; `total_amount = items_subtotal + shipping_fee - discount` is built from it.
  - Phase B: the checkout first calls `CommitFlashSaleStock(reservation_id)` for every flash hold it placed, then `commitDomainReservations`, before any order row exists. Order matters: a flash commit error or `committed=false` fails with the domain reservations still uncommitted (existing compensation path): `fail` runs `failAndCompensate` (stock, voucher and flash holds released) and the checkout returns `UNAVAILABLE`. A domain commit failure after the flash commits releases the flash holds through `failAndCompensate` (promotion returns committed units). A crash before compensation strands nothing: promotion expires an uncommitted hold after 15 minutes.
  - `failAndCompensate` releases the flash holds placed by the attempt (the ids recorded during Phase A, only flash lines; committed ones too, promotion returns the units), best effort, background context, same budget as the voucher release. `CancelOrder` releases after the claim for **every item** of the cancelled order, because the order item stores no price source: `ReleaseFlashSaleStock` on an id that never had a hold is a no-op in promotion, and a committed hold's units are returned (`stock_sold` decreases). A release error is logged and never fails the cancel. Every checkout attempt generates fresh order ids, so a reservation id is never reused after its hold was released (a replay of a released hold would return `reserved=false`, `not_active`).
  - Handler (`internal/handler/order.go`): `ErrPriceChanged` maps to `FAILED_PRECONDITION` with message exactly "price changed"; `ErrPromotionUnavailable` maps to `UNAVAILABLE`.
- **team-order, settle**: no change. `consumer/payment.go` does not touch flash holds (the hold is already committed in checkout Phase B).
- **Proto**: `proto/platform/promotion/v1/promotion.proto` copied from `platform-core/packages/proto` (the contract slice's version) and Go stubs regenerated; never hand-edited.
- **platform-e2e** (owned by this slice): new `features/order/advertised_price.feature` and steps, plus a flash-campaign helper in `support/` (seeded through `FlashSaleService.CreateCampaign` as admin).
- Not changed: team-promotion, team-gateway (service-only RPCs; `CreateOrder`/`AddToCart` routes exist), the frontend, `CalculateShippingFee`, order item schema (no price source), multi-shop shipping or voucher scoping (Q11).

## NFR

- Latency: `AddToCart` adds one `GetActiveFlashSale` call. Checkout adds, before any hold, one `GetListing` per selected line and one `GetActiveFlashSale` per distinct listing, plus one `ReserveFlashSaleStock` per flash-priced line. Each call is bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (existing config, default 2 s = 2000 ms), no retries; worst-case added latency before any hold is therefore bounded by (lines + distinct listings) x 2 s, and checkout caps lines per order at the cart size seen today (no new limit is added). Phase B adds one commit call per flash line. `CancelOrder` adds one release call per order item (no price source is stored), same 2 s bound each, parked in the existing inline release budget; failure never fails the cancel. A reservation_id is `uuid:listing id:variant id`, 36 + 2 + ids bytes, under the contract's 200-byte limit.
- Availability: promotion down or unset never charges an unconfirmed price. Add-to-cart degrades to the regular price. Checkout of regular-priced lines is not blocked by a promotion outage; a flash-priced line blocks its checkout with `UNAVAILABLE`.
- Limits: amounts are int64 VND, no rounding. At most `stock_cap` units are sold at the flash price (enforced by promotion's hold; this slice only calls it).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| flash reservation id (`order id:listing id:variant id`) | operational, pseudonymous (order id) | idempotent hold, commit and release | contract performance (order processing) | in promotion until commit, release or 15-minute expiry of an uncommitted hold; committed holds follow promotion's sales records; no copy stored in team-order |
| quantity, expected_sale_price | operational | hold size and price guard | same | same |
| cart `unit_price` re-price | operational | show the buyer the current price | same | existing cart retention |

No new personal data. Buyer id is not sent in any hold RPC.

## Threat model

- **Assets:** the charged amount of every order, flash stock, the promotion service credential (`service-team-order`, scope `promotion.reserve`).
- **Abuse cases:**
  - Buyer keeps a stale flash price in the cart after the sale ended: checkout re-resolves and refuses, the cart is re-priced.
  - Buyer carts before a sale starts to hold the old higher price, or after to keep a lower one: any difference refuses and re-prices.
  - Concurrent checkouts oversell the campaign: each flash line takes a hold; promotion refuses beyond `stock_cap`; the refused buyer is re-priced to the regular price.
  - Holds leaked by a failed or cancelled checkout: released in `failAndCompensate` and `CancelOrder`, idempotent; a crash before release is covered by the 15-minute expiry of uncommitted holds.
  - A browser holding or releasing campaign stock: the RPCs are service-only in promotion; team-order sends them as its own service principal and never forwards a buyer principal on them.
  - Promotion outage used to get an unconfirmed price: a flash-priced line fails closed (`UNAVAILABLE`), a missing flash price never raises the charge above the regular price.
  - Another buyer's cart re-priced: re-pricing is scoped by `user_id` and item id from the caller's own cart.
- **Accepted risk:** placed but unpaid orders count as sold flash stock until cancelled (unpaid-order expiry is backlog Q20).
- **Authz rule:** `AddToCart`, `CreateOrder` and `Reorder` keep their existing buyer-owned authz (principal id = cart owner / order owner); this slice adds no new RPC and no new scope for callers.
