@buyer
Feature: Product detail page - variants, purchase feedback, mobile buy bar and anchors
  The PDP is rebuilt on the core components (ui-phase-product-detail). Variant
  selection lives in the URL and drives price and stock; Thêm vào giỏ / Mua ngay
  give pending, disabled and toast feedback; a sticky buy bar serves 375px; the
  body is stacked sections with an anchor nav; an unknown id renders a product
  not-found result. Needs the local stack (a seeded seller and listing).

  @needsSeller
  Scenario: Selecting a variant with a different price updates the URL and the price
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing
    And the buyer selects the variant "256GB"
    Then the URL carries the "256GB" variant id without an extra history entry
    And the price shows "1.500.000"
    And the stock line shows "3 sản phẩm"

  @needsSeller
  Scenario: A shared variant URL restores the selection
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing with the "256GB" variant in the URL
    Then the "256GB" variant is selected
    And the price shows "1.500.000"
    And the stock line shows "3 sản phẩm"

  @needsSeller
  Scenario: An unknown variant id falls back safely
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing with an unknown variant in the URL
    Then the "128GB" variant is selected
    And the "512GB" variant is disabled with an out-of-stock tag

  @needsSeller
  Scenario: Add to cart shows pending then success
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing
    And the add-to-cart request is slowed down
    And the buyer increases the quantity 1 times
    And the buyer clicks Thêm vào giỏ
    Then both purchase buttons are disabled and the clicked one is busy
    And a toast "Đã thêm 2 sản phẩm vào giỏ hàng" appears
    And both purchase buttons are enabled again
    And the cart counter shows 2

  @needsSeller
  Scenario: Quantity cannot exceed stock
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing with the "256GB" variant in the URL
    And the buyer increases the quantity 4 times
    Then the quantity is 3 and the increase control is disabled

  @needsSeller
  Scenario: Buy now adds the selected variant then goes to checkout
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing with the "256GB" variant in the URL
    And the buyer clicks Mua ngay
    Then the browser navigates to "/checkout"
    And the cart contains the "256GB" variant

  @needsSeller
  Scenario: Only one set of purchase buttons is visible per breakpoint
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the viewport is 375 pixels wide
    And the buyer opens the variant listing
    And the buyer scrolls to the reviews section
    Then the buy bar is visible with the price "1.000.000"
    And exactly one Thêm vào giỏ button is visible
    When the viewport is 1280 pixels wide
    Then no buy bar is visible
    And exactly one Thêm vào giỏ button is visible

  @needsListing
  Scenario: Anchor links point at the sections
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    Then the anchor nav links to the specs, reviews and Q&A sections
    When the buyer follows the anchor link "Đánh giá"
    Then the reviews section is in view

  @needsListing
  Scenario: A listing with no reviews and no sale shows no invented numbers
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    Then the rating row reads "Chưa có đánh giá"
    And no sold count, Mall badge or strike-through price is shown

  Scenario: An unknown listing renders the product not-found result
    When the visitor opens the unknown listing "does-not-exist"
    Then the product not-found result offers the home page and search
