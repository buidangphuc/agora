@seller @fintech
Feature: Seller Order Fulfillment and Wallet Payout
  As a seller
  I want to process order fulfillment with SPX Express
  And view my revenue analytics to request wallet payouts

  @smoke @needsSeller @needsListing
  Scenario: Seller views analytics and requests wallet payout
    Given I am logged in as a seller via API
    When I navigate to the "seller analytics" page
    Then I should see the revenue metric cards
    And I should see the seller wallet balance
