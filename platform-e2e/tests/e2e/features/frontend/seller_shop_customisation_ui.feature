@seller @shop @needsSeller
Feature: Seller brands the shop from /seller/shop (seller-shop-customisation-ui)
  The seller sets banner, tagline, featured listings and theme in the browser; scenario names echo
  the e2e scenarios of openspec/changes/seller-shop-customisation-ui. Data is seeded through the
  gateway API; the server rejection of a foreign featured listing is driven through the gateway.

  Scenario: Seller shop UI: a seller brands the shop in the browser and the buyer sees it
    Given a seeded seller is logged in
    And the seller owns two published listings for the shop
    When the seller brands the shop in the browser with a banner, tagline "Hoa tươi mỗi ngày", both listings in order and theme "dark"
    Then a success toast confirms the shop information
    When the seller reloads the shop form
    Then the shop form shows the same banner, tagline "Hoa tươi mỗi ngày", both listings in the chosen order and theme "dark"
    And the storefront API returns the banner, tagline "Hoa tươi mỗi ngày", both listings in the chosen order and theme "dark"
    And the buyer app shop page shows the tagline "Hoa tươi mỗi ngày"

  Scenario: Seller shop UI: a foreign featured listing is rejected
    Given a seeded seller is logged in
    And the seller owns two published listings for the shop
    And the seller has saved a storefront featuring both listings with tagline "Hoa tươi mỗi ngày"
    And a second seller owns a published listing for the shop
    When the seller saves a storefront featuring the second seller's listing through the gateway
    Then the save fails with invalid_argument
    And the storefront API still returns tagline "Hoa tươi mỗi ngày" and both listings in the original order
    And the buyer app shop page shows no listing of the second seller
