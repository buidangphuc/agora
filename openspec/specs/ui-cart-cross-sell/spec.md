# ui-cart-cross-sell Specification

## Purpose
TBD - created by archiving change recsys-cart-cross-sell. Update Purpose after archive.

## Requirements

### Requirement: The cart shows a cross-sell row

The `/cart` page SHALL show a "Thường được mua cùng" row of at most 8 listings from placement `cart_cross_sell`, sending `context_listing_ids` holding the distinct listing ids of at most 50 cart lines (the last 50 in the order `GetCart` returns, which are the 50 newest because `team-order/internal/store/cart.go:65` orders lines `created_at ASC`) as `context_listing_ids`. The row SHALL hide itself when the cart is empty, the result is empty or the call errors, and the cart SHALL still render in those cases. Its beacons SHALL carry `impressionId` equal to the response `request_id` and the response `model_version`.

#### Scenario: Row shows for a cart with items

- **WHEN** a buyer with items in the cart opens `/cart`
- **THEN** the page shows the "Thường được mua cùng" row with 1 to 8 listings, none of them a cart line
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_cross_sell_ui.feature` `Row shows for a cart with items`

#### Scenario: Row is hidden for an empty cart

- **WHEN** a buyer with an empty cart opens `/cart`
- **THEN** no "Thường được mua cùng" row is rendered and no Recommend call is made
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_cross_sell_ui.feature` `Row is hidden for an empty cart`

#### Scenario: Cart renders when recommendations fail

- **WHEN** a buyer opens `/cart` while the Recommend call returns an error
- **THEN** the cart lines and totals render and the row is absent
- **VERIFIED BY**: `team-frontend/src/app/(shop)/cart/crossSell.test.tsx` `cart renders when recommendations fail`

#### Scenario: Row is hidden for an empty result

- **WHEN** Recommend returns zero items for a non-empty cart
- **THEN** the row is absent and the cart renders
- **VERIFIED BY**: `team-frontend/src/app/(shop)/cart/crossSell.test.tsx` `row is hidden for an empty result`

#### Scenario: At most 50 lines are sent

- **WHEN** the cart holds 60 lines and `/cart` renders
- **THEN** the Recommend request carries exactly the last 50 line listing ids in the order the cart returned them
- **VERIFIED BY**: `team-frontend/src/app/(shop)/cart/crossSell.test.tsx` `at most 50 lines are sent`

#### Scenario: Beacons carry the recommendation identity

- **WHEN** the row is visible and the buyer clicks an item
- **THEN** the impression and click beacons carry `impressionId` equal to the response `request_id` and the response `model_version`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/cart_cross_sell_ui.feature` `Beacons carry the recommendation identity`
