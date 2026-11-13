## ADDED Requirements

### Requirement: A published bundle is added to the cart as tagged lines

`CartService.AddBundleToCart` SHALL read the bundle with `ListingService.GetBundle`, check every listing, and add
one cart line per listing tagged with the `bundle_id`, each with quantity `quantity`, the listing price as
`unit_price`, and a `bundle_discount` allocated pro rata to the line prices so that the line discounts sum exactly to
`(sum of unit prices - bundle price) * quantity`. The caller SHALL be authenticated and the cart SHALL be the
caller's own. The request SHALL carry only `bundle_id` and `quantity` (1..99).

#### Scenario: Adding a bundle creates one tagged line per listing

- **WHEN** a buyer calls `AddBundleToCart` for a published bundle of listings priced 100000 and 50000 with bundle price 120000 and quantity 1
- **THEN** the returned cart holds two lines tagged with the bundle id whose `bundle_discount` values are 20000 and 10000 respectively
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Adding a bundle creates one tagged line per listing`

#### Scenario: The discount is allocated exactly with the remainder on the last line

- **WHEN** a buyer calls `AddBundleToCart` with quantity 3 for a bundle whose discount per combo is 10000 over three listings priced 10000, 10000 and 10000
- **THEN** the three `bundle_discount` values sum to exactly 30000 and `Cart.bundle_discount_total` equals 30000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `The discount is allocated exactly with the remainder on the last line`

#### Scenario: Adding the same bundle again increases every line

- **WHEN** a buyer who already has bundle B in the cart at quantity 1 calls `AddBundleToCart` for B with quantity 2
- **THEN** every line of B has quantity 3 and `Cart.bundle_discount_total` equals three times the per-combo discount
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Adding the same bundle again increases every line`

#### Scenario: A bundle line stays separate from a plain line of the same listing

- **WHEN** a buyer who has listing X as a plain line adds a bundle that contains X
- **THEN** the cart holds a plain line for X and a bundle-tagged line for X, and the plain line keeps its quantity and price
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A bundle line stays separate from a plain line of the same listing`

#### Scenario: An unknown bundle is rejected

- **WHEN** a buyer calls `AddBundleToCart` with a bundle id that does not exist
- **THEN** the call fails with NOT_FOUND and the cart is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `An unknown bundle is rejected`

#### Scenario: A quantity out of range is rejected

- **WHEN** a buyer calls `AddBundleToCart` with quantity 0, and again with quantity 100
- **THEN** both calls fail with INVALID_ARGUMENT and the cart is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A quantity out of range is rejected`

#### Scenario: A bundle with an unavailable listing is refused

- **WHEN** a buyer calls `AddBundleToCart` for a bundle in which one listing is unpublished, or out of stock for the requested quantity, or not owned by the bundle seller
- **THEN** each call fails with FAILED_PRECONDITION and no line is added to the cart
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A bundle with an unavailable listing is refused`

#### Scenario: A bundle with a variant listing is refused

- **WHEN** a buyer calls `AddBundleToCart` for an existing bundle that contains a listing with variants
- **THEN** the call fails with FAILED_PRECONDITION and no line is added to the cart
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A bundle with a variant listing is refused`

#### Scenario: A bundle priced at or above the sum of its listings is refused

- **WHEN** a buyer calls `AddBundleToCart` for a bundle whose price is not below the sum of its current listing prices
- **THEN** the call fails with FAILED_PRECONDITION and no line is added to the cart
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A bundle priced at or above the sum of its listings is refused`

#### Scenario: An unauthenticated caller is rejected

- **WHEN** a caller without a session calls `AddBundleToCart` through the gateway
- **THEN** the call fails with UNAUTHENTICATED and no cart is read or written
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `An unauthenticated caller is rejected`

#### Scenario: The cart of another buyer is never touched

- **WHEN** buyer A adds a bundle and buyer B then calls `GetCart`
- **THEN** buyer B's cart holds no line tagged with that bundle id
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `The cart of another buyer is never touched`

### Requirement: Changing a bundle line drops the bundle from the cart

`UpdateCartItem` (any quantity change) and `RemoveFromCart` on a bundle line SHALL remove the `bundle_id` and
`bundle_discount` from every line of that bundle in the caller's cart, so regular prices apply, and the cart
`subtotal` SHALL be net of the remaining bundle discounts.

#### Scenario: Changing one line quantity drops the tag from all lines

- **WHEN** a buyer with bundle B in the cart calls `UpdateCartItem` on one of its lines with a different quantity
- **THEN** no line of B keeps a `bundle_id` or `bundle_discount` and `Cart.bundle_discount_total` is 0
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Changing one line quantity drops the tag from all lines`

#### Scenario: Removing one line drops the tag from the others

- **WHEN** a buyer with bundle B in the cart calls `RemoveFromCart` on one of its lines
- **THEN** the removed line is gone, the remaining lines of B are plain lines at regular prices and `Cart.bundle_discount_total` is 0
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Removing one line drops the tag from the others`

#### Scenario: A dropped line merges into the plain line of the same listing

- **WHEN** a buyer has a plain line of listing X at quantity 1 and a bundle line of X at quantity 2, then changes the quantity of another line of that bundle
- **THEN** the cart holds one line for X with quantity 3 and no bundle tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A dropped line merges into the plain line of the same listing`

#### Scenario: Another buyer's bundle line cannot be changed

- **WHEN** buyer B calls `UpdateCartItem` or `RemoveFromCart` with the id of a bundle line in buyer A's cart
- **THEN** the call fails with NOT_FOUND and buyer A's bundle tag is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Another buyer's bundle line cannot be changed`

### Requirement: Checkout prices a valid bundle at its combo price

`PreviewCheckout` and `CreateOrder` SHALL re-read every bundle present in the selected cart lines. WHEN the bundle is
valid, all its lines are selected with equal quantities and the stored discount matches the recomputed one, the
quote and the order SHALL use the allocated line prices, `Order.total_amount` SHALL reflect the bundle price, and the
order items SHALL carry `bundle_id` and `bundle_discount`. OTHERWISE the bundle tag SHALL be dropped from its cart
lines and the call SHALL fail with the existing price-changed error before any stock, voucher or payment is touched.

#### Scenario: The order total reflects the bundle price

- **WHEN** a buyer checks out a cart holding only a valid two-listing bundle (prices 100000 and 50000, bundle price 120000, quantity 1) with a shipping fee of 0
- **THEN** the order `total_amount` is 120000 and `PreviewCheckout` returned the same items subtotal of 120000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `The order total reflects the bundle price`

#### Scenario: The seller's order shows the bundle on its lines

- **WHEN** the seller of a bundle order calls `GetOrder` for that order
- **THEN** every item carries the bundle id and a `bundle_discount` summing to 30000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `The seller's order shows the bundle on its lines`

#### Scenario: A bundle that became invalid is untagged before payment

- **WHEN** a listing of the bundle is unpublished after the buyer added it and the buyer calls `PreviewCheckout`
- **THEN** the call fails with the price-changed error, the cart lines of the bundle are plain at regular prices, and no stock or voucher reservation exists
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A bundle that became invalid is untagged before payment`

#### Scenario: A changed bundle price is untagged at checkout

- **WHEN** the seller changes the listing prices so the recomputed discount differs from the stored one and the buyer calls `CreateOrder`
- **THEN** the call fails with the price-changed error and no order is created
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A changed bundle price is untagged at checkout`

#### Scenario: Checking out only part of a bundle drops the discount

- **WHEN** a buyer selects only one line of a two-line bundle for `CreateOrder`
- **THEN** the call fails with the price-changed error and both lines of the bundle are plain lines at regular prices
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `Checking out only part of a bundle drops the discount`

#### Scenario: A seller cannot check out their own bundle

- **WHEN** the seller of a bundle adds it to their own cart and calls `CreateOrder`
- **THEN** the call fails with the existing self-purchase error and no order is created
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A seller cannot check out their own bundle`

#### Scenario: A return refunds the allocated bundle price

- **WHEN** the buyer requests a return without an amount on a paid bundle order with `total_amount` 120000
- **THEN** the return `refund_amount` is 120000 and a request above 120000 fails with INVALID_ARGUMENT
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A return refunds the allocated bundle price`

### Requirement: Existing carts and orders are unchanged by the bundle migration

The migration SHALL be additive: existing cart and order rows SHALL have `bundle_id` empty and `bundle_discount` 0, and
the plain `AddToCart`, `GetCart`, `PreviewCheckout` and `CreateOrder` paths SHALL price lines exactly as before.

#### Scenario: A plain cart is priced as before

- **WHEN** a buyer with no bundle lines adds a listing with `AddToCart` and calls `PreviewCheckout`
- **THEN** `Cart.bundle_discount_total` is 0, the subtotal equals `unit_price * quantity` and the preview totals equal those of the previous release
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/bundle_checkout.feature` `A plain cart is priced as before`

#### Scenario: Re-running the migration changes nothing

- **WHEN** migration `0025_bundle_lines` is applied twice to a database with existing cart and order rows
- **THEN** the second run reports no error and no row changes its `bundle_id` or `bundle_discount`
- **VERIFIED BY**: `team-order/migrations/migration_test.go` `TestBundleLinesMigrationIdempotent`
