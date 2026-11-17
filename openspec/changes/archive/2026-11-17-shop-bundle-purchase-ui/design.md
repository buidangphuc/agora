## Context

`ShopPage` already loads `listBundlesBySeller(params.id)` and the shop's published listings (`searchListings`), and
renders the combo block inline in `page.tsx`. Cart lines are `ViewCartItem`s mapped in `src/lib/gateway/cart.ts` and
rendered by `CartGroups`; cart mutations are server actions in `src/features/cart/actions.ts` that `revalidatePath`
`/cart` and `/checkout`.

## Decisions

- The inline combo block moves into `BundleCard` (server component) plus a client island `AddBundleButton`. `page.tsx`
  only passes `bundles` and the already loaded shop listings; `shop-page-featured-theme` lands first and its
  featured/theme changes stay outside the combo block, so the merge touches disjoint lines.
- Display availability is a pure helper `bundleAvailability(bundle, shopListings)`: unavailable when any
  `listingId` is missing from the shop's published listings (the search result is already filtered by seller and
  published). Savings are shown only when all listing prices are known. Out-of-stock and variant cases are NOT
  guessed on the client; they surface when `AddBundleToCart` answers `FAILED_PRECONDITION`, which flips the card.
  Why: the server is the only authority (initiative decision), no extra reads on render.
- `addBundleToCartAction(bundleId)` mirrors `addToCartAction`: returns `ok`, `unauthenticated`, or a typed failure
  `unavailable` (`FAILED_PRECONDITION`), `notFound` (`NOT_FOUND`), otherwise a generic message. It revalidates `/cart`
  and `/checkout`.
- Cart display: `groupByShop` keeps lines in order and additionally groups consecutive/same-`bundleId` lines under a
  combo header inside the shop group. Line total shown = `unitPrice * quantity - bundleDiscount` as returned; the
  group subtotal and the "Giảm giá combo" row use server values (`bundleDiscountTotal`). No client price math beyond
  that single subtraction for a line label.
- Bundle-drop notice: the quantity and remove controls pass the line's `bundleId` to their actions; the action
  result carries `bundleDropped: true` when the line was bundled, and the control shows the notice via
  `role="status"`. The server performs the actual drop (initiative decision); the UI never rewrites prices itself.
- Unauthenticated add: same redirect to `/login?next=/shop/{id}` as the listing add-to-cart flow.
- No feature flag and no in-memory fallback: a failed call shows an error.

## Risks

- Contract fields `bundle_discount` semantics (per line total vs per unit) are specified in `seller-centre-contract`
  as the allocated share for the line; this slice treats it as the line total. Flagged in open issues.
- `shop/[id]/page.tsx` is shared with `shop-page-featured-theme`; this slice is built after it and edits only the
  combo block.
