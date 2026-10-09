@notification @chat
Feature: Chat delivery survives faults and failed writes publish nothing
  notification-delivery-hardening: team-chat writes chat.events through an outbox,
  team-notification keeps its dedupe ledger in Postgres, and a chat notification is still
  created when the sender-name lookup fails.

  # Stops the shared redpanda container: serial lane only. The seller's reply is stored
  # while the relayer cannot publish; after Kafka returns the outbox relays it once.
  @destructive @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: A message sent while Kafka is down is still delivered
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And Kafka is stopped
    And the seller replies to the buyer inquiry
    Then the buyer sees the seller's reply in the conversation thread
    When Kafka is started again
    Then the buyer receives exactly one chat notification for the seller's reply

  # A write that fails leaves nothing behind. The black-box probe is a SendMessage the
  # service rejects (caller outside the thread): no message, no event, no notification.
  # The post-outbox-insert rollback itself is covered by team-chat's Postgres test.
  @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: A failed message write publishes nothing
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And a user outside the thread tries to send a message into it
    Then the message is rejected and is not stored in the thread
    When the buyer sends a follow-up chat message
    Then the seller has a chat notification for the follow-up
    And neither participant has a chat notification for the rejected message

  # Rewinds team-notification's chat consumer group to the start while it is stopped, so
  # every chat.events record (including the reply) is delivered again after the restart;
  # the durable ledger must keep the buyer at one notification.
  @destructive @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: A redelivered event after a restart notifies once
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And the seller replies to the buyer inquiry
    Then the buyer has a chat notification for the seller's reply
    When team-notification is stopped and its chat consumer group is rewound to the start
    And team-notification is running again and has replayed chat.events
    Then the buyer still has exactly one chat notification for the seller's reply

  # Stops team-identity before the buyer writes: the buyer is not the thread's seller, so
  # the sender's name can only come from identity; the seller's notification still arrives,
  # titled with the neutral label.
  @destructive @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: Name lookup failure still notifies
    Given the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the sender-name lookup is unavailable
    And the buyer sends a chat inquiry to the seller regarding the order
    Then the seller's chat notification is created with the neutral sender label
