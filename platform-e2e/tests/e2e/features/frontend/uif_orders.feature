@buyer @order @destructive
Feature: Buyer order failure paths with a real service outage (ui-phase-orders)
  Each scenario genuinely stops team-order (it backs ListBuyerOrders and GetOrder) and restores it
  in teardown. Destructive: serial lane only.

  Scenario: A failed load shows an Alert with retry
    Given a d2 buyer has 1 orders from one shop
    When team-order is stopped to force a real failure
    And the d2 buyer opens the orders list
    Then an error alert with a retry button replaces the order list and the empty state is not shown
    When team-order is running again and answers through the gateway
    And the buyer presses the retry button of the alert once
    Then the buyer's order is listed again

  Scenario: A transport failure offers retry
    Given a d2 buyer has 1 orders from one shop
    When team-order is stopped to force a real failure
    And the d2 buyer opens the order detail
    Then an error alert with a retry button is shown and no 404 result is rendered
    When team-order is running again and answers through the gateway
    And the buyer presses the retry button of the alert once
    Then the order detail is rendered again
