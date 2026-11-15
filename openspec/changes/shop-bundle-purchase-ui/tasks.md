## 1. Code track (team-frontend) - tests first

- [x] 1.1 Regenerate/sync the proto client after `seller-centre-contract`; `src/lib/gateway/cart.ts`: `ViewCartItem.bundleId/bundleDiscount`, `ViewCart.bundleDiscountTotal`, `addBundleToCart(bundleId, quantity)` (C1, C8)
- [x] 1.2 `src/features/cart/actions.test.ts` (red first): `addBundleToCartAction reports unauthenticated`, `addBundleToCartAction maps not found and outage`, failed precondition maps to `unavailable` (C3, C4, C6)
- [x] 1.3 `src/features/shop/BundleCard.test.tsx`: `shows the saving`, unavailable when a listing is missing from the shop listings, no button when unavailable (C2, C7)
- [x] 1.4 `src/features/shop/AddBundleButton.test.tsx`: `sends one request on double click`, `flips to unavailable on failed precondition`, success link to `/cart`, `role="alert"` on error (C3, C5, C6, C1)
- [x] 1.5 `groupByShop.test.ts`: `treats lines without bundle id as regular`, combo grouping; `CartGroups.test.tsx`: `renders no combo UI without bundle lines`, combo label and per-line discount (C8, C9, C11)
- [x] 1.6 Implement `bundleAvailability` helper, `BundleCard.tsx`, `AddBundleButton.tsx`, `addBundleToCartAction`; replace the inline combo block in `src/app/(shop)/shop/[id]/page.tsx` (combo block only; featured/theme code from `shop-page-featured-theme` untouched) (C1-C7)
- [x] 1.7 `groupByShop.ts`, `CartGroups.tsx`, "Giảm giá combo" row on `/cart` and the `/checkout` summary; bundle-drop notice from `CartQuantityControl.tsx` and the remove action (`bundleDropped`) (C8, C10, C12, C13)
- [ ] 1.8 `team-frontend` `npm run check` green

## 2. E2E track (platform-e2e, team-frontend/FEATURES.yaml)

- [x] 2.1 `tests/e2e/features/frontend/shop_bundle_purchase_ui.feature`: scenarios "Buyer adds a combo to the cart", "A combo with an unpublished listing is unavailable", "Cart shows the combo discount", "Removing a combo line cancels the combo", "Checkout total reflects the bundle price" (C1, C2, C8, C10, C12)
- [x] 2.2 Steps seed through the real gateway API (seller registers, creates and publishes listings, `CreateBundle`, unpublishes one listing for C2); buyer signs in through the UI; no route fulfils (C1, C2, C8, C10, C12)
- [x] 2.3 `team-frontend/FEATURES.yaml` entries for `/shop/[id]` combo purchase and `/cart` combo lines, `covered_by` the feature above (C1, C8)
- [ ] 2.4 `openspec validate shop-bundle-purchase-ui --strict`, `spec_lint.py`, features check, pre-commit-ci
