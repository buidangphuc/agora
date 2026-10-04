@buyer @engagement
Feature: Product Reviews Breakdown & Filters
  As a buyer
  I want to view star ratings breakdown and customer reviews
  So that I can evaluate customer satisfaction before purchasing

  @needsListing
  Scenario: Buyer inspects product star rating breakdown and review filters
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    Then the listing page displays the reviews breakdown section and rating filter buttons

  # ui-phase-product-detail: the star filter and the page live in the URL.
  Scenario: Filtering reviews by stars is shareable through the URL
    Given a buyer is viewing a listing with reviews of several star values
    When the buyer clicks the "4 Sao" review filter
    Then the URL contains rating=4 and only 4-star reviews are listed
    And reloading the page shows the same filtered list

  Scenario: A star filter with no matching review offers a way back to all reviews
    Given a buyer is viewing a listing with reviews of several star values
    When the buyer opens the reviews filtered by 2 stars
    Then an empty state "Không có đánh giá 2 sao" is shown
    When the buyer follows "Xem tất cả" in the reviews empty state
    Then the URL no longer contains a rating filter
