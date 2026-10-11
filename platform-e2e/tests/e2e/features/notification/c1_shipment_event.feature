@notification @order
Feature: The OrderShipped event is atomic with the shipment and harmless to analytics
  notify-chat-and-shipment: team-order writes OrderShipped with the shipment, and the
  other order.events consumers skip it.

  # Black-box probe of "a failed shipment write publishes nothing": a CreateShipment the
  # service rejects (a seller who does not own the order) stores no shipment and the buyer
  # is never notified. The mid-transaction rollback is covered by team-order's Postgres test.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: The shipment event is written with the shipment
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When another seller tries to create a shipment for the order
    Then the shipment is rejected and the order is not shipped
    And the buyer has no order notification for the rejected shipment

  # team-analytics' order-facts consumer reads order.events too; it must skip OrderShipped
  # and keep advancing (a consumer that errored would be stuck behind the event).
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Order analytics ignores the new event
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    And the order.events end offset is noted
    When the seller fulfills the shipment with tracking information
    Then within 30 seconds team-analytics' order consumer has committed past the OrderShipped event
    And team-analytics is still running
