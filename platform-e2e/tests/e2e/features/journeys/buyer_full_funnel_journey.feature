@buyer @smoke @tracking @search @promo
Feature: Buyer Full Funnel Journey
  As a buyer on Agora marketplace, I want to browse the home page, search and filter, open a
  product page, favorite and share it, build a cart, redeem a voucher and place an order, with
  the GA4 dataLayer reporting the purchase.

  # Every step drives the running stack (the Next.js UI through Playwright, the gateway API for
  # the seeding and the read-backs) and every Then reads the result back:
  #   * home impressions: window.dataLayer `view_item_list` for a card on the page;
  #   * search: the price facet is active in the URL and the seeded listing is in the grid;
  #   * favorite: ListFavorites; share link: ResolveShareLink + opening /s/<code>;
  #   * cart: GetCart quantities and subtotal, the quantity field on /cart;
  #   * voucher: the checkout page's discount and total rows (SAVE10 = 10%, platform voucher);
  #   * order: ListBuyerOrders + GetOrder; GA4: the `purchase` event in window.dataLayer, whose
  #     transaction_id is that order id.
  # The backend today: SearchListings reports no retrieval mode, so "hybrid" is not observable and
  # the step claims only keyword search + facet; the checkout does not render an order
  # confirmation page, it lands on the buyer's order list (/account/orders?success=1).

  @needsBuyer @needsListing @needsAddress
  Scenario: Complete buyer full funnel journey from search to checkout and GA4 purchase
    Given I am logged in as a buyer via API
    And a listing has been seeded via the API
    When I navigate to the "home" page
    Then a viewable impression event is emitted to the data layer for a home listing card
    When the buyer searches for the seeded listing and filters by its price range
    Then the filtered search results include the seeded listing
    When the buyer opens the seeded listing from the search results
    Then the product detail page displays the seeded listing's title and price
    When the buyer favorites the listing and generates a share link
    Then the listing is a favorite of the buyer and the share link resolves to the listing
    When the buyer adds the listing to the cart and raises its quantity to 2
    Then the cart holds 2 units of the listing
    When the buyer proceeds to checkout
    And the buyer applies the voucher code "SAVE10" at checkout
    Then the voucher discount is shown and the order total is reduced
    When the buyer confirms the order placement
    Then the order is placed with the voucher discount and listed for the buyer
    And a purchase event for that order is pushed to the GA4 dataLayer

  # serve-trained-recs-locally: team-ai serves the trained generation (RECS_ENABLED=true, Qdrant backend),
  # so the home row renders cards and its row impressions reach the data layer.
  @needsBuyer
  Scenario: Home page shows AI recommendations with viewable impressions
    Given I am logged in as a buyer via API
    When I navigate to the "home" page
    Then the "Gợi ý cho bạn" row on the home page shows product cards
    And a viewable impression event is emitted to the data layer for the recommendations row
