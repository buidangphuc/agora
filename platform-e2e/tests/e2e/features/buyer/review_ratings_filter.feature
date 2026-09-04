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
