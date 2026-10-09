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

  @needsSeller
  Scenario: Payout is disabled without balance
    Given I am logged in as a seller via API
    When the seller opens the seller wallet page
    Then the payout button is disabled and says why

  @needsSeller @needsOrder
  Scenario: Payout asks for confirmation before sending anything
    Given the buyer has paid the seeded order to the seller
    And I am logged in as a seller via API
    When the seller opens the seller wallet page
    And the seller starts a payout
    Then a confirm dialog asks for the payout amount and nothing is sent yet
