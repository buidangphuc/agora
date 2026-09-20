@buyer @smoke @recommendations @tracking @search @promo
Feature: Buyer Full Funnel Journey
  As a buyer on Agora marketplace, I want to experience a seamless end-to-end shopping journey
  starting with homepage AI recommendations and viewable telemetry, performing hybrid search and
  filter refinements, engaging on the PDP with favorites and link sharing, managing multi-item cart
  quantities, applying promotional voucher discounts, and completing checkout with verified GA4 dataLayer events.

  @needsBuyer @needsListing @needsAddress
  Scenario: Complete buyer full funnel journey from AI recommendations to checkout and GA4 confirmation
    Given a buyer is logged in
    And a listing has been seeded via the API
    When I navigate to the "home" page
    Then the "Gợi ý cho bạn" recommendations row is populated with product cards
    And a viewable impression telemetry event is emitted to the data layer
    When the buyer searches for "E2E" with hybrid search and applies filters
    Then the search results grid updates matching the filtered criteria
    When the buyer opens the seeded listing
    Then the product detail page displays product details
    When the buyer favorites the listing and generates a share link
    Then the product is marked as favorite and a valid share link is created
    When the buyer adds multiple items to the cart
    Then the cart contains the updated item quantities
    When the buyer proceeds to checkout
    And the buyer applies the voucher code "SAVE10" at checkout
    Then the voucher discount is shown and the order total is reduced
    When the buyer confirms the order placement
    Then the order confirmation is displayed and a purchase event is pushed to the GA4 dataLayer
