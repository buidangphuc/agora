@buyer @order
Feature: Order Tracking Timeline and RMA Refund
  As a buyer
  I want to track my order delivery milestones
  And be able to request an RMA refund if I changed my mind

  @smoke @needsBuyer @needsListing
  Scenario: Buyer tracks delivery milestones and submits an RMA refund request
    Given I am logged in as a buyer via API
    And I have an active order with SPX shipment tracking
    When I navigate to the "account orders" page
    Then I should see the order delivery timeline
    When I submit an RMA refund request with reason "changed_mind"
    Then I should see the RMA refund success confirmation
