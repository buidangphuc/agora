## 1. Code track (team-order, team-gateway)

- [ ] 1.1 Failing tests first for add, allocation, tag drop, checkout guard (scenarios: adding a bundle creates one tagged line per listing; the discount is allocated exactly with the remainder on the last line; adding the same bundle again increases every line; a quantity out of range is rejected)
- [ ] 1.2 `migrations/0025_bundle_lines.{up,down}.sql` and store changes: bundle columns on cart and order items, key `(user_id, listing_id, variant_id, bundle_id)` (scenarios: a bundle line stays separate from a plain line of the same listing; re-running the migration changes nothing; a plain cart is priced as before)
- [ ] 1.3 `CartService.AddBundleToCart` in `internal/order/cart.go` and `internal/rpc/cart.go`: GetBundle, concurrent GetListing, owner/status/variant/stock/price checks, pro-rata allocation, atomic write (scenarios: an unknown bundle is rejected; a bundle with an unavailable listing is refused; a bundle with a variant listing is refused; a bundle priced at or above the sum of its listings is refused; an unauthenticated caller is rejected; the cart of another buyer is never touched)
- [ ] 1.4 Tag drop in `UpdateCartItem`/`RemoveFromCart` with merge into the plain line (scenarios: changing one line quantity drops the tag from all lines; removing one line drops the tag from the others; a dropped line merges into the plain line of the same listing; another buyer's bundle line cannot be changed)
- [ ] 1.5 Bundle step in `precheckPrices` shared by `PreviewCheckout` and `CreateOrder`; net line totals in subtotal, quote and order items (scenarios: the order total reflects the bundle price; the seller's order shows the bundle on its lines; a bundle that became invalid is untagged before payment; a changed bundle price is untagged at checkout; checking out only part of a bundle drops the discount; a seller cannot check out their own bundle; a return refunds the allocated bundle price)
- [ ] 1.6 team-gateway `internal/edge/cart.go` forwarder for `AddBundleToCart` and its edge test (scenario: an unauthenticated caller is rejected)
- [ ] 1.7 No raw bundle or buyer ids in logs (sloglint) (scenarios: all above)

## 2. e2e track (platform-e2e, team-order/FEATURES.yaml)

- [ ] 2.1 `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` with one scenario per spec scenario, bundles and listings seeded through the real gateway (scenarios: all)
- [ ] 2.2 Step definitions and seeding helper for bundles, listing unpublish and price change (scenarios: a bundle that became invalid is untagged before payment; a changed bundle price is untagged at checkout)
- [ ] 2.3 `team-order/FEATURES.yaml` entries for `order.v1.AddBundleToCart` and the bundle checkout journey, `covered_by` the feature scenarios (scenarios: the order total reflects the bundle price)

## 3. Verification

- [ ] 3.1 `openspec validate order-bundle-checkout --strict`, `spec_lint.py`, gates through `gate_digest.py` with `GO_SDK_REQUIRE_DOCKER=1`
