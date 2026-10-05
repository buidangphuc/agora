@buyer @seller @chat @order @notification
Feature: Post-Purchase Chat, Notification, and RMA Return Journey
  As a buyer and seller on Agora marketplace, we want chat communication regarding orders,
  notifications on milestone updates, shipment fulfillment, and an RMA return workflow.

  # Every step drives the running gateway as the buyer or the seller and reads the
  # result back (chat thread RPCs, GetOrder, GetShipmentTracking, GetReturnRequest).
  # The backend today: a COD order stays PENDING; the seller can ship it from any
  # status (it becomes SHIPPED); a SHIPPED order can be returned; approving a return
  # only moves the return to APPROVED (team-order does not call team-payment).

  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Post-purchase buyer seller chat, order tracking, and RMA return flow
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    Then the chat message is delivered in the conversation thread
    When the seller replies to the buyer inquiry
    Then the buyer sees the seller's reply in the conversation thread
    When the seller fulfills the shipment with tracking information
    Then the order is shipped with that tracking code
    When the buyer submits an RMA return request for the order
    Then the RMA return request is created with pending status
    When the seller approves the RMA return request
    Then the RMA return request is approved

  # GAP (expected to fail): team-chat publishes chat.events only to the gateway SSE edge
  # and team-notification only consumes listing.events, so no CHAT notification exists.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Seller reply notifies the buyer
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And the seller replies to the buyer inquiry
    Then the buyer has a chat notification for the seller's reply

  # GAP (expected to fail): team-order publishes only OrderPaidEvent and nothing
  # consumes it into an ORDER notification, so shipping notifies nobody.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Shipping the order notifies the buyer
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the seller fulfills the shipment with tracking information
    Then the buyer has an order notification for the shipment
