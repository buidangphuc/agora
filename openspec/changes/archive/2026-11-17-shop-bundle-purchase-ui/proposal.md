## Why

Initiative `seller-centre` story 4: the "Combo tiết kiệm" block on `/shop/[id]` (`team-frontend`) shows a bundle
title, item count and bundle price but offers no way to buy it, and the cart and checkout screens know nothing about
bundles. `order-bundle-checkout` makes `CartService.AddBundleToCart` real (cart lines tagged `bundle_id`, discount
allocated per line, order priced at the bundle price); this slice is the buyer UI on top of it.

initiative: seller-centre

## What Changes

- Each combo card on `/shop/[id]` gets a "Thêm combo vào giỏ" button, plus the listing titles it contains and the
  saving ("Tiết kiệm X VND" = sum of the combo listings' current prices minus the bundle price) when every listing is
  known to the page.
- A combo is rendered as unavailable (no button, text "Combo tạm không khả dụng") when any of its listings is not
  among the shop's published listings the page already loads (unpublished, deleted, foreign), or when
  `AddBundleToCart` answers `FAILED_PRECONDITION` (the card flips to unavailable without a reload).
- Clicking the button calls `CartService.AddBundleToCart(bundle_id, quantity=1)` through a server action; success
  shows an "Đã thêm combo" confirmation with a link to `/cart` and refreshes the cart badge. A visitor without a
  session is sent to login with a return path to the shop page, as the existing add-to-cart flow does.
- `/cart` shows bundle lines grouped under a "Combo" label per `bundle_id` inside the shop group, each line with its
  allocated `bundle_discount`, and a "Giảm giá combo" row with `Cart.bundle_discount_total`. Totals shown are the
  ones returned by the server; the browser does not compute prices.
- Removing a bundle line or changing its quantity shows the notice "Combo đã bị huỷ, các sản phẩm tính giá thường"
  after the server drops the bundle tag from all lines of that bundle.
- `/checkout` order summary shows the same "Giảm giá combo" row from the cart/preview data; no new checkout RPC.
- Gateway helper `addBundleToCart` in `team-frontend/src/lib/gateway/cart.ts`; `ViewCartItem` gains `bundleId` and
  `bundleDiscount`, `ViewCart` gains `bundleDiscountTotal`.
- Consumes the contract of `seller-centre-contract` (generated client with `AddBundleToCart`, `CartItem.bundle_id`,
  `CartItem.bundle_discount`, `Cart.bundle_discount_total`). No new RPC, no proto or backend change.

## Capabilities

### New Capabilities
- `ui-shop-bundle-purchase`: buying a combo from the shop page and seeing it in cart and checkout.

### Modified Capabilities
none

## Impact

- `team-frontend/src/app/(shop)/shop/[id]/page.tsx` (hot file, also edited by `shop-page-featured-theme`, built first),
  new `src/features/shop/BundleCard.tsx` and `AddBundleButton.tsx`, `src/features/cart/actions.ts`,
  `CartGroups.tsx`, `CartQuantityControl.tsx`, `groupByShop.ts`, `src/lib/gateway/cart.ts`, checkout summary,
  `team-frontend/FEATURES.yaml`, the vendored proto copy if `npm run` proto sync requires it.
- `platform-e2e` feature `frontend/shop_bundle_purchase_ui.feature` plus steps.

## Non-goals

- No bundle creation or editing UI (seller app), no bundle pricing rules (`domain-bundle-integrity`,
  `order-bundle-checkout`).
- No bundle quantity stepper on the shop page: the button adds quantity 1; quantity is changed in the cart.
- No seller-side rendering of bundle lines on orders (seller app is outside this slice).
- No variant bundles: a bundle with a variant listing is refused by the server and shown unavailable after the failed add.

## Non-functional requirements

- Shop page server render stays within today's budget plus 50 ms locally with 10 bundles: availability is computed
  from the shop listing result already loaded by the page, with no per-bundle or per-listing extra call.
- "Thêm combo vào giỏ" round trip p95 <= 400 ms locally (server action plus `AddBundleToCart` p95 <= 300 ms for
  <= 10 listings).
- The button is disabled while a request is in flight, so a double click adds the combo once.
- Degradation: if `ListBundlesBySeller` fails the combo block is absent (as today); a failed add shows an inline
  error and leaves the page usable.
- Accessible: button has a visible label naming the combo, status messages use `role="status"`/`role="alert"`.

## Data classification

- Bundle title, listing ids and bundle price: seller-provided public shop content (purpose: shop page and combo
  sale, basis: contract, retention: while the shop exists). Cart lines and discounts: the buyer's own business data
  (purpose: purchase, basis: contract, retention: with the cart/order, owned by team-order). No new personal data is
  collected, stored or logged by this slice; buyer and seller ids are not logged raw.

## Threat model

- Assets: price integrity of a combo order; the buyer's cart.
- Abuse cases: (1) a forged client request sending a bundle price or discount: the action sends only `bundle_id` and
  quantity, the server computes every price and the UI renders server values only; (2) adding a bundle of another
  shop's listings or a stale bundle: refused by the server (`FAILED_PRECONDITION`), the UI never decides
  availability for the purchase, only for display; (3) hostile bundle title: rendered as escaped React text;
  (4) repeated clicks flooding the cart: button disabled while in flight and the server action requires a session.
- Authz: the shop page is a public read; adding to cart requires a buyer session (`Unauthenticated` leads to login);
  a buyer only ever touches their own cart (server derives the user from the principal).
