# ui-shop-bundle-purchase Specification

## Purpose
Defines the buyer UI for buying a bundle: adding a combo to the cart from the shop page and showing bundle lines and the bundle discount in cart and checkout.

## Requirements

### Requirement: Buy a combo from the shop page

`/shop/[id]` SHALL show a "Thêm combo vào giỏ" button on every available combo card, which calls
`CartService.AddBundleToCart` with the bundle id and quantity 1. A combo SHALL be rendered as unavailable, without a
button, when any of its listings is absent from the shop's published listings or when the add is refused with
`FAILED_PRECONDITION`. The browser SHALL send no price or discount.

#### Scenario: C1 Adding an available combo puts every listing in the cart

- **WHEN** a signed-in buyer clicks "Thêm combo vào giỏ" on a combo of 2 published listings on `/shop/{id}`
- **THEN** the cart contains 2 lines, one per listing, each tagged with that bundle, and the page shows "Đã thêm combo" with a link to `/cart`

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature` `C1 Adding an available combo puts every listing in the cart`

#### Scenario: C2 A combo with an unpublished listing is shown unavailable

- **WHEN** a visitor opens `/shop/{id}` and one listing of a combo has been unpublished
- **THEN** that combo card shows "Combo tạm không khả dụng" and has no "Thêm combo vào giỏ" button

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature` `C2 A combo with an unpublished listing is shown unavailable`

#### Scenario: C3 A refused add flips the card to unavailable

- **WHEN** the server answers `FAILED_PRECONDITION` to the add of a combo that the page showed as available
- **THEN** that card changes to "Combo tạm không khả dụng" without a reload and the cart is unchanged

- **VERIFIED BY**: `team-frontend/src/features/shop/AddBundleButton.test.tsx` `flips to unavailable on failed precondition`

#### Scenario: C4 A guest is sent to login

- **WHEN** a visitor without a session clicks "Thêm combo vào giỏ"
- **THEN** THE SYSTEM SHALL redirect to `/login` with a return path to `/shop/{id}` and add nothing to the cart

- **VERIFIED BY**: `team-frontend/src/features/cart/actions.test.ts` `addBundleToCartAction reports unauthenticated`

#### Scenario: C5 A double click adds the combo once

- **WHEN** a buyer double-clicks "Thêm combo vào giỏ" within one request round trip
- **THEN** the button is disabled after the first click and exactly one `AddBundleToCart` request is sent

- **VERIFIED BY**: `team-frontend/src/features/shop/AddBundleButton.test.tsx` `sends one request on double click`

#### Scenario: C6 An unknown bundle or an outage shows an error

- **WHEN** `AddBundleToCart` answers `NOT_FOUND` or fails with an unavailable-service error
- **THEN** the card shows an inline `role="alert"` message, no "Đã thêm combo" text, and the page stays interactive

- **VERIFIED BY**: `team-frontend/src/features/cart/actions.test.ts` `addBundleToCartAction maps not found and outage`

#### Scenario: C7 The combo card shows the saving when all prices are known

- **WHEN** a combo of listings priced 100000 and 50000 VND has a bundle price of 120000 VND and all its listings are in the shop's published listings
- **THEN** the card shows "Tiết kiệm 30.000 VND" and lists both listing titles

- **VERIFIED BY**: `team-frontend/src/features/shop/BundleCard.test.tsx` `shows the saving`

### Requirement: Cart shows combo lines and the combo discount

`/cart` SHALL group the lines that share a `bundle_id` under a "Combo" label inside their shop group, show each
line's `bundle_discount`, and show a "Giảm giá combo" row with `Cart.bundle_discount_total` when it is greater
than zero. The totals SHALL be the server-returned values. When a bundle line is removed or its quantity changed, the
cart SHALL show "Combo đã bị huỷ, các sản phẩm tính giá thường".

#### Scenario: C8 Combo lines and the discount are shown in the cart

- **WHEN** a buyer opens `/cart` holding a combo of 2 lines with a bundle discount total of 30000 VND
- **THEN** both lines appear under one "Combo" label and the summary shows "Giảm giá combo" with 30.000 VND

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature` `C8 Combo lines and the discount are shown in the cart`

#### Scenario: C9 A cart without bundles is unchanged

- **WHEN** a buyer opens `/cart` holding only regular lines (bundle discount total 0)
- **THEN** the page has no "Combo" label and no "Giảm giá combo" row

- **VERIFIED BY**: `team-frontend/src/features/cart/CartGroups.test.tsx` `renders no combo UI without bundle lines`

#### Scenario: C10 Removing one combo line shows the cancellation notice

- **WHEN** a buyer removes one line of a bundle or changes its quantity
- **THEN** the cart shows "Combo đã bị huỷ, các sản phẩm tính giá thường" and no "Combo" label remains for the remaining lines

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature` `C10 Removing one combo line shows the cancellation notice`

#### Scenario: C11 A combo line with a missing discount does not break the cart

- **WHEN** the gateway returns a bundle line with an empty `bundle_id` or a `bundle_discount` of 0
- **THEN** THE SYSTEM SHALL render that line as a regular line, without a "Combo" label

- **VERIFIED BY**: `team-frontend/src/features/cart/groupByShop.test.ts` `treats lines without bundle id as regular`

### Requirement: Checkout shows the combo discount

The `/checkout` order summary SHALL show a "Giảm giá combo" row with the server-returned bundle discount total when it
is greater than zero, and SHALL show the order total returned by the server.

#### Scenario: C12 The order total reflects the bundle price

- **WHEN** a buyer with a combo in the cart (regular sum 150000 VND, bundle price 120000 VND) opens `/checkout`
- **THEN** the summary shows "Giảm giá combo" 30.000 VND and a merchandise total of 120.000 VND

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_bundle_purchase_ui.feature` `C12 The order total reflects the bundle price`

#### Scenario: C13 An invalid combo at checkout shows regular prices

- **WHEN** the server dropped the bundle tag because the bundle became invalid (a listing was unpublished) and the buyer opens `/checkout`
- **THEN** the summary has no "Giảm giá combo" row and the merchandise total is the sum of regular line prices returned by the server

- **VERIFIED BY**: `team-frontend/src/features/cart/CartGroups.test.tsx` `renders no combo UI without bundle lines`
