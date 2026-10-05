@notification @chat
Feature: Notification delivery hardening
  Chat notifications name the sender, and team-notification keeps its state in
  Postgres, so a restart neither loses the price baseline nor re-notifies.

  # team-chat publishes chat.events through its outbox with the thread's seller_id;
  # team-notification names the sender via team-domain (shop) or team-identity (user).
  @buyer @seller @needsBuyer @needsSeller @needsListing @needsAddress @needsOrder
  Scenario: A seller reply names the shop
    Given the seller sets the shop display name to "Nhà Sách An Nhiên"
    And the buyer who placed the order is logged in
    And an order has been placed and is pending fulfillment
    When the buyer sends a chat inquiry to the seller regarding the order
    And the seller replies to the buyer inquiry
    Then the buyer's chat notification title contains "Nhà Sách An Nhiên"

  # The last-seen price lives in listing_last_seen, so the restarted consumer still
  # has the 2,000,000 baseline when the price drops to 500,000.
  @buyer @destructive
  Scenario: A price drop after a team-notification restart is still detected
    Given a buyer subscribed to a "price_drop" alert on a seeded listing
    And the notification consumer has recorded the listing's price
    When team-notification is restarted
    And the seller lowers the listing price
    Then a "price_drop" notification appears in the notifications center
