@order
Feature: Order Detail & Delivery Timeline
  As a buyer
  I want to view my order details and live shipping progression
  So that I know the status and delivery estimate of my package

  @needsListing
  Scenario: Buyer inspects order details and 5-step delivery timeline
    Given a buyer is logged in
    And the buyer has placed an order via API
    When the buyer navigates to the order detail page
    Then the order summary shows the 5-step delivery timeline and items
