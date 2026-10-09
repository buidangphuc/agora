@buyer @search
Feature: Discovery routes - cards, grid, search, vouchers and the search bar
  ui-phase-discovery: the listing card and grid, real-data-only rendering, the URL-driven search
  page, vouchers and the search bar. Needs the local stack: listings are seeded through the gateway
  and waited for in the search index; every assertion is on the rendered page.

  # ── Listing card and grid ─────────────────────────────────────────────
  Scenario: Card reserves its image box
    Given listings with images are indexed under one keyword
    And the listing images load slowly
    When the buyer opens the search results while the images are still loading
    Then every card image container is already a 1:1 box
    And no card changes height when its image loads

  Scenario: Image fallback keeps the layout
    Given listings without a working image are indexed under one keyword
    When the buyer opens the search results for that keyword
    Then every card shows the placeholder inside a 1:1 box

  Scenario: Below-the-fold images are lazy
    Given 26 listings with images are indexed under one keyword
    When the buyer opens the search results for that keyword
    Then the images of cards 1-6 are not lazy and the images of cards 7-24 are lazy

  Scenario: Empty grid offers a way out
    Given a keyword that matches no listing
    When the buyer opens the search results for that keyword
    Then an Empty block says nothing was found and suggests changing keywords or filters

  Scenario: Listing without reviews shows no rating or sold count
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword
    Then no card shows a rating, a rating number or a sold count

  Scenario: Card shows only the real price
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword
    Then every card shows exactly one price and no strike-through price, discount badge or MALL badge

  Scenario: Mall is never guessed
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword
    Then the card of the listing above 5000000 and the card with the brand keyword show no MALL badge

  # ── Real data only ────────────────────────────────────────────────────
  Scenario: Unsupported features are hidden
    Given a voucher has been seeded via the gateway
    And the "vouchers" page is open
    Then no voucher card has a "Lưu mã" button
    When the buyer opens the search results page
    Then the sort options exclude "Bán Chạy"

  Scenario: Token lint passes on discovery files
    When the token lint runs on the discovery files
    Then the token lint reports no violation in the discovery files

  Scenario: Hub tiles are not rainbow
    Given the "home" page is open
    Then the eight service hub tiles share one neutral surface

  Scenario: No campaign data means no fake countdown or sold bar
    Given the "home" page is open
    Then the home page has no flash-sale heading, countdown, sold bar or discount badge

  # ── Category navigation, search page state ────────────────────────────
  # DEFECT (xfail in the binder): CategoryBar's pills variant is not mounted on /search - the
  # category is only a facet link in the filter sidebar - so there is no pill row above the results.
  Scenario: Current category is announced
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword in the "cat-electronics" category
    Then the current category pill is marked aria-current and the "Tất cả" pill is not

  Scenario: Changing a filter resets the page
    Given 26 listings with images are indexed under one keyword
    When the buyer opens page 2 of the results and selects the "Trên 1.000.000₫" price filter
    Then the new URL carries the filter and no page

  Scenario: Single page hides Pagination
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword
    Then no Pagination is rendered

  Scenario: Invalid price range is rejected
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the buyer opens the search results for that keyword
    And the buyer enters a minimum of 500000 and a maximum of 100000 and applies
    Then the maximum field shows an error, the URL does not change and the apply button is not loading

  # ── Mutations ─────────────────────────────────────────────────────────
  Scenario: Save search shows pending then toast
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    And the form submission is slowed down
    When the buyer opens the search results for that keyword
    And the buyer activates "Lưu tìm kiếm này"
    Then the button is pending and disabled, then a success toast appears and the saved list contains the keyword

  Scenario: Save search with empty query is rejected
    When the buyer opens the search results page
    Then "Lưu tìm kiếm này" is disabled so no request can be sent and nothing is saved

  # ── Vouchers ──────────────────────────────────────────────────────────
  Scenario: No voucher save without a backend
    Given a voucher has been seeded via the gateway
    And the browser storage is observed
    When the visitor opens the vouchers page
    Then no card has a "Lưu mã" or "Đã lưu" control and no voucher key was read from or written to localStorage

  Scenario: Seller tools are scope gated
    Given a voucher has been seeded via the gateway
    When a buyer opens the vouchers page
    Then the voucher manager is not rendered
    When a seller opens the vouchers page
    Then the voucher manager is rendered

  Scenario: Vouchers grid at 375px
    Given a voucher has been seeded via the gateway
    And the viewport is a 375px wide phone
    When the visitor opens the vouchers page
    Then voucher cards are one per row and the page does not scroll sideways

  # ── Search bar ────────────────────────────────────────────────────────
  Scenario: Submit navigates to search
    Given the "home" page is open
    When the visitor types "ao khoac" in the search bar and presses Enter
    Then the browser navigates to "/search?q=ao%20khoac"

  Scenario: Empty submit is ignored
    Given the "home" page is open
    When the visitor presses Enter in the empty search bar
    Then the browser stays on the home page

  Scenario: Suggestion failure is silent
    Given the "home" page is open
    And the suggestion endpoint fails
    When the visitor types "ao" in the search bar
    Then the suggestion list is hidden, no toast appears and the input keeps working

  Scenario: Keyboard selects a suggestion
    Given listings priced 1250000 and 6000000 with a brand keyword are indexed
    When the visitor types the keyword in the search bar and presses ArrowDown then Enter
    Then the first suggestion is submitted as the query

  # ── Tracking ──────────────────────────────────────────────────────────
  Scenario: Attribution survives the Suspense rework
    Given a buyer with the home page open while recording tracking beacons
    When the recommendations row has streamed in, the buyer scrolls to it and clicks its first card
    Then both the impression and the click beacon carry placementId "home_feed"
