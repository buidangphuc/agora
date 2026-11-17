## Why

A combo shown on `/shop/[id]` carries a bundle price, but nothing can buy it: `CartItem` and `OrderItem` have no
bundle reference (order.proto), `AddToCart` prices every line at its listing price, and checkout, preview and
returns know nothing about a bundle. The seller-centre initiative (story 4, Q27) makes a bundle purchasable at its
combo price. This slice is the team-order and team-gateway half: `CartService.AddBundleToCart`, the bundle tag on
cart and order lines, the pro-rata discount in checkout totals, and the guards that drop the tag when the bundle no
longer holds.

initiative: seller-centre

## What changes

- team-order implements `CartService.AddBundleToCart` (proto added by `seller-centre-contract`): it reads the bundle
  with `ListingService.GetBundle`, reads every listing with `ListingService.GetListing`, checks each one (owner =
  bundle seller, PUBLISHED, no variants, stock >= quantity, bundle price < sum of listing prices), and adds one cart
  line per listing tagged `bundle_id`, with the discount `(sum of unit prices - bundle price) * quantity` allocated
  pro rata to the line prices (rounding remainder on the last line).
- Migration `0025_bundle_lines`: `cart_items.bundle_id` and `bundle_discount`, `order_items.bundle_id` and
  `bundle_discount`; the cart uniqueness key becomes `(user_id, listing_id, variant_id, bundle_id)` so a bundle line
  never merges into a plain line of the same listing.
- `UpdateCartItem` (any quantity change) and `RemoveFromCart` on a bundle line drop the tag from every line of that
  bundle; the cart reports `bundle_discount_total` and the lines show regular prices again.
- `PreviewCheckout` and `CreateOrder` re-read each bundle in the selected lines. A bundle that is invalid, changed,
  or only partly selected loses its tag (cart lines and quote use regular prices) and the call answers with the
  existing price-changed error before anything is reserved or paid. A valid bundle is priced at the allocated line
  prices; `Order.total_amount`, the seller's order items (`bundle_id`, `bundle_discount`) and the return cap use them.
- team-gateway registers `CartService.AddBundleToCart` in the cart forwarder (`internal/edge/cart.go`); it needs a
  buyer session like the other cart RPCs and is not an admin or seller RPC.
- Cart subtotal is net of the bundle discount (`subtotal = sum(unit_price * quantity) - bundle_discount_total`).
- Flash-sale prices are not applied to bundle lines; a bundle line is priced at the listing's regular price.

## Capabilities

### New Capabilities
- `order-bundle-checkout`: buying a published bundle at its combo price through cart, preview and checkout.

### Modified Capabilities
none

## Impact

- `team-order/internal/order/cart.go`, `order.go`, `checkoutquote.go`, `flashprice.go` (precheck), `internal/store/cart.go`,
  `internal/store/order.go`, `internal/rpc/cart.go`, `internal/rpc/order.go`, `migrations/0025_bundle_lines.{up,down}.sql`,
  `team-order/FEATURES.yaml`.
- `team-gateway/internal/edge/cart.go` (hot file shared with `promotion-seller-self-service`: serialised by
  initiative-build).
- `platform-e2e` features for the order API scenarios.
- Depends on `seller-centre-contract` (proto) and `domain-bundle-integrity` (CreateBundle refuses foreign listings,
  fewer than 2 listings, price >= sum, variant listings). The UI is `shop-bundle-purchase-ui`.

## Non-goals

- No change to `CreateBundle`/`GetBundle` (team-domain) and no bundle storage in team-order beyond the line tag.
- No flash-sale stacking, no voucher change: shop and platform vouchers apply to the bundle-discounted subtotal
  exactly as to any subtotal.
- No bundle with variant listings; no bundle across sellers.
- No per-item partial return rule: the return cap stays the order total, which already holds the bundle price.

## Non-functional requirements

- `AddBundleToCart` p95 <= 300 ms locally for a bundle of up to 10 listings (one `GetBundle` plus `GetListing`
  calls issued concurrently, at most 10 in flight).
- Bundles hold 2..10 listings; `quantity` is 1..99 per call.
- Availability: a down team-domain fails the call (UNAVAILABLE); there is no in-memory fallback and no enable flag.
- The migration is additive and idempotent; existing cart and order rows keep `bundle_id = ''`, `bundle_discount = 0`.
- Checkout totals stay integer minor units; allocated line discounts sum exactly to the bundle discount.

## Data classification

No new personal data. `bundle_id` and `bundle_discount` are order/cart line attributes (purpose: price the purchase;
basis: contract; retention: with the cart line and, once ordered, with the order record under the existing order
retention). Bundle ids and buyer ids are not logged raw (sloglint forbidden keys); logs carry counts and codes only.

## Threat model

- Assets: order totals (money), seller stock, the bundle price integrity.
- Abuse cases: (1) buyer keeps a bundle tag after the bundle became invalid to pay the combo price (mitigated: checkout
  re-reads the bundle and drops the tag); (2) buyer changes one line quantity or checks out only part of a bundle to
  keep the discount (mitigated: tag dropped from all lines); (3) seller edits a bundle to include a foreign listing
  or an inflated price (mitigated: owner check at add and in `domain-bundle-integrity`); (4) client supplies a
  discount or bundle price (mitigated: the request carries only `bundle_id` and `quantity`; the discount is computed
  in team-order); (5) buyer adds another buyer's bundle line (cart is principal-scoped).
- Authz rule: `AddBundleToCart` requires an authenticated principal (UNAUTHENTICATED otherwise); the cart is always
  the caller's own; any buyer may add any published bundle; a seller adding their own bundle is allowed in the cart
  and refused at checkout by the existing self-purchase rule.
