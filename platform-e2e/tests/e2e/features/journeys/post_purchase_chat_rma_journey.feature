@buyer @seller @chat @order @notification
Feature: Post-Purchase Chat, Notification, and RMA Return Journey
  As a buyer and seller on Agora marketplace, we want real-time chat communication regarding orders,
  instant notification delivery upon milestone updates, efficient shipment fulfillment, and an automated
  RMA return and refund workflow.

  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Post-purchase buyer seller chat, order tracking notification, and RMA return flow
    Given a buyer is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    Then the chat message is delivered in the conversation thread
    When the seller replies to the buyer inquiry
    Then the buyer receives a real-time message notification
    When the seller fulfills the shipment with tracking information
    Then the order status transitions to shipped and a delivery notification is recorded
    When the buyer submits an RMA return request for the order
    Then the RMA return request is created with pending status
    When the seller approves the RMA return request
    Then the RMA return request is approved and refund processing is initiated
