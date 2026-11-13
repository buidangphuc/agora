@buyer @seller
Feature: A buyer buys a seller combo from the shop page and sees it in the cart (shop-bundle-purchase-ui)
  Sellers, listings and combos are seeded through the gateway API; the buyer signs in through the UI
  and buys through /shop/{id}, /cart and /checkout. Scenario names echo the e2e scenarios of
  openspec/changes/shop-bundle-purchase-ui (initiative seller-centre); the unit-verified scenarios
  (C3-C7, C9, C11, C13) are not-testable entries in team-frontend/FEATURES.yaml.

  Scenario: C1 Adding an available combo puts every listing in the cart
    Given a bsp seller with a published combo of listings priced 100000 and 50000 at combo price 120000
    And a bsp buyer with a saved address signs in through the UI
    When the bsp buyer opens the seller's shop page once the combo can be added
    And the bsp buyer clicks "Thêm combo vào giỏ"
    Then the bsp page shows "Đã thêm combo" with a link to "/cart"
    And the bsp buyer's cart holds one line per combo listing, each tagged with the combo

  Scenario: C2 A combo with an unpublished listing is shown unavailable
    Given a bsp seller with a published combo of listings priced 100000 and 50000 at combo price 120000
    And the second listing of the bsp combo is unpublished
    When a bsp visitor opens the seller's shop page once the combo shows unavailable
    Then the bsp combo card shows "Combo tạm không khả dụng" and has no "Thêm combo vào giỏ" button

  Scenario: C8 Combo lines and the discount are shown in the cart
    Given a bsp seller with a published combo of listings priced 100000 and 50000 at combo price 120000
    And a bsp buyer with a saved address signs in through the UI
    And the bsp buyer has the combo in the cart
    When the bsp buyer opens the cart
    Then the bsp cart shows both combo lines under one "Combo" label
    And the bsp cart summary shows "Giảm giá combo" with 30000

  Scenario: C10 Removing one combo line shows the cancellation notice
    Given a bsp seller with a published combo of listings priced 100000 and 50000 at combo price 120000
    And a bsp buyer with a saved address signs in through the UI
    And the bsp buyer has the combo in the cart
    When the bsp buyer opens the cart
    And the bsp buyer removes the first combo line
    Then the bsp cart shows "Combo đã bị huỷ, các sản phẩm tính giá thường"
    And the bsp cart has no "Combo" label left

  Scenario: C12 The order total reflects the bundle price
    Given a bsp seller with a published combo of listings priced 100000 and 50000 at combo price 120000
    And a bsp buyer with a saved address signs in through the UI
    And the bsp buyer has the combo in the cart
    When the bsp buyer opens the checkout page
    Then the bsp checkout summary shows "Giảm giá combo" with 30000 and a merchandise total of 120000
