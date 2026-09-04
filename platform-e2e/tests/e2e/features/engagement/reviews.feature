@buyer @needsBuyer @needsListing @needsOrder
Feature: Verified star reviews
  As a buyer, I can leave star reviews on purchased products.

  Scenario: Buyer submits verified star review with rating
    Given a buyer has purchased a product
    When the buyer submits a star review
    Then the review and updated rating appear on the listing
