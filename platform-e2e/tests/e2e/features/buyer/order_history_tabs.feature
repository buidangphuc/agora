@buyer @order
Feature: Buyer Order History
  As a buyer
  I want to view all my previous purchases on my orders page
  So that I can review past order statuses, items and totals

  @needsListing
  Scenario: Buyer views order history cards
    Given a buyer is logged in
    And the buyer has placed an order via API
    When the buyer navigates to the orders page
    Then the orders list displays the placed order cards with status badges
