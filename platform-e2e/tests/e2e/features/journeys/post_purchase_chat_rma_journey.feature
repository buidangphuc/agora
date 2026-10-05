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

  # team-chat publishes the message to chat.events with the other participant as
  # recipient_id; team-notification turns it into a CHAT notification for that user only.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Seller reply notifies the buyer
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And the seller replies to the buyer inquiry
    Then the buyer has a chat notification for the seller's reply
    And the seller has no chat notification for their own reply

  # chat.events is keyed by thread, so one consumer handles a thread's messages in
  # order: once the seller is notified of the follow-up (sent after the reply), the
  # reply has certainly been processed, so "no notification" is not a vacuous pass.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Chat notifications respect preferences
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    And the buyer has disabled chat notifications
    When the buyer sends a chat inquiry to the seller regarding the order
    And the seller replies to the buyer inquiry
    And the buyer sends a follow-up chat message
    Then the seller has a chat notification for the follow-up
    And the buyer has no chat notification for the seller's reply

  # team-order writes OrderShipped to its outbox with the shipment; team-notification
  # turns it into an ORDER notification for the buyer.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Shipping the order notifies the buyer
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the seller fulfills the shipment with tracking information
    Then the buyer has an order notification for the shipment
