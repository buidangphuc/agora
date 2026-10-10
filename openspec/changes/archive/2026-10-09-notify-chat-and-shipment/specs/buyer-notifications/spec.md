## Purpose

Defines which marketplace events create an in-app notification for the user they concern,
beyond listing alerts (price drop, back in stock).

## ADDED Requirements

### Requirement: A chat message notifies the other participant

When a message is sent in a chat thread, `team-notification` SHALL create one
`NOTIFICATION_TYPE_CHAT` notification for the thread participant who did not send it,
linking to that thread (`/chat/<thread_id>`), with the sender's display label as the title
and the start of the message as the body. The sender SHALL NOT be notified of their own
message. A redelivered event SHALL NOT create a second notification. The notification
SHALL be skipped when the recipient has disabled `NOTIFICATION_TYPE_CHAT` in their
notification preferences.

#### Scenario: Seller reply notifies the buyer

- **WHEN** a buyer asks a question in a thread and the seller replies
- **THEN** within 30 s the buyer's `ListNotifications` contains one `NOTIFICATION_TYPE_CHAT`
  notification linking to that thread, and the seller has none for their own reply

#### Scenario: Chat notifications respect preferences

- **WHEN** the buyer has disabled `NOTIFICATION_TYPE_CHAT` and the seller replies
- **THEN** no chat notification is created for the buyer

### Requirement: Shipping an order notifies the buyer

When a seller creates a shipment for an order, `team-order` SHALL write an `OrderShipped`
event (order id, buyer id, seller id, carrier, tracking number, shipped_at) to its outbox in
the same transaction as the shipment, relayed to `order.events` keyed by order id.
`team-notification` SHALL consume it and create one `NOTIFICATION_TYPE_ORDER` notification
for the buyer, linking to `/account/orders/<order_id>` and naming the carrier and tracking
number. A redelivered event SHALL NOT create a second notification, and the notification
SHALL be skipped when the buyer has disabled `NOTIFICATION_TYPE_ORDER`. Existing consumers of
`order.events` SHALL keep working (they ignore event types they do not handle).

#### Scenario: Shipping the order notifies the buyer

- **WHEN** the seller creates a shipment with a tracking code for the buyer's order
- **THEN** within 30 s the buyer's `ListNotifications` contains one `NOTIFICATION_TYPE_ORDER`
  notification linking to that order and containing the tracking code

#### Scenario: The shipment event is written with the shipment

- **WHEN** creating the shipment fails after the outbox row would have been written
- **THEN** neither the shipment nor the `OrderShipped` outbox row is stored

#### Scenario: Order analytics ignores the new event

- **WHEN** an `OrderShipped` event is published to `order.events`
- **THEN** team-analytics' order-facts consumer skips it and its offset advances
