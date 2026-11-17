## Context

team-order is the only price authority for a purchase (no cross-DB join). The cart stores `unit_price` per line
and checkout re-derives advertised prices in `precheckPrices` before any saga starts; a mismatch re-prices the cart
and answers the price-changed error with nothing reserved. Bundles live in team-domain (`GetBundle`).

## Decisions

1. Line shape. One cart line per bundle listing with `bundle_id` and `bundle_discount` (the line's total discount,
   already multiplied by quantity). `unit_price` stays the listing price, so the line total is
   `unit_price * quantity - bundle_discount`. Order items copy both fields; `itemsSubtotal`, `Cart.subtotal` and the
   quote use the net line total. Why: keeps unit prices truthful for display and for the existing advertised-price
   check, and keeps one rule for the return cap (order total).
2. Quantity. `AddBundleToCart.quantity` (1..99, otherwise INVALID_ARGUMENT) is the number of combos: every line gets
   quantity `q`. Total discount `D = (sum(price_i) - bundle_price) * q`; line i gets `floor(D * price_i / sum(price_i))`,
   the last line takes the remainder, so the allocated discounts sum exactly to `D`. Adding the same bundle again
   increases every line by `q` and recomputes the allocation from the current data.
3. Separate line key. The unique key gains `bundle_id` so a plain line of listing X and a bundle line of X coexist.
   When a tag is dropped and the line collides with a plain line, quantities are summed into the plain line.
4. Add-time checks, in order: bundle exists (NOT_FOUND), >= 2 listings, every `GetListing` ok, owner equals
   `bundle.seller_id`, status PUBLISHED, no variants, `stock >= quantity`, `bundle_price < sum(price_i)`; any failure is
   FAILED_PRECONDITION (a down team-domain is UNAVAILABLE). Nothing is written when one check fails (all or none, one
   transaction for the line writes). Flash prices are ignored: lines use the regular listing price.
5. Tag-drop rules. `UpdateCartItem` (even to the same quantity, any change) or `RemoveFromCart` on a bundle line
   clears `bundle_id` and `bundle_discount` on all lines with that `bundle_id` for the caller in one transaction;
   a removed line is deleted, the others become plain lines. The response cart reports it via
   `bundle_discount_total` falling to 0 for that bundle; the UI owns the message.
6. Checkout guard. `precheckPrices` gets a bundle step: for each distinct `bundle_id` among the selected lines
   it re-reads the bundle and listings and applies the add-time checks, and also requires that the selected lines are
   exactly the bundle's listings with equal quantities and that the stored discount equals the recomputed one. If not,
   the bundle's lines in the cart are untagged (plain prices) and the existing price-changed error is returned
   before the saga, the reservations or any voucher hold. `PreviewCheckout` runs the same step, so the preview shows
   regular prices before payment. A valid bundle proceeds at the stored allocated discounts.
7. Gateway. One forwarder method in `edge/cart.go`; the edge policy treats it like the other cart RPCs (session
   required). No admin policy change.
8. No flags, no fallback: a down dependency fails the call.

## Risks

- Stock is checked at add and again by the existing reservation in the saga; there is no hold at add time.
- A bundle price edit between add and checkout is caught by the stored-versus-recomputed discount comparison.
