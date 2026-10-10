# buyer-notifications Specification

## Purpose
Defines which events notify a buyer (chat replies, shipments, price drops), how a notification names its sender, and that delivery survives restarts, broker outages and failed name lookups without duplicates.

## Requirements

### Requirement: Chat message events are written through an outbox

`team-chat` SHALL write the message-sent event (with its recipient) to an outbox table in the
same database transaction as the message, and a relayer SHALL publish it to `chat.events`
keyed by thread id, preserving creation order within a claimed batch. A message that is
stored SHALL eventually produce its event even if Kafka was unavailable when it was sent;
a message that fails to store SHALL produce no event.

#### Scenario: A message sent while Kafka is down is still delivered

- **WHEN** a seller replies while the relayer cannot reach Kafka, and Kafka comes back
- **THEN** the event is published after recovery and the buyer receives the chat
  notification once

#### Scenario: A failed message write publishes nothing

- **WHEN** storing the message fails after the outbox row would have been written
- **THEN** neither the message nor its outbox row is stored

### Requirement: Notification consumers keep their state across restarts

`team-notification` SHALL record processed event ids (per consumer) and the listing
consumer's last-seen price and stock in its own Postgres database, so that a restart or
redeploy neither re-notifies a redelivered event nor loses the baseline needed to detect a
price drop or a restock.

#### Scenario: A redelivered event after a restart notifies once

- **WHEN** team-notification restarts after creating a notification but before committing
  the Kafka offset, and the event is redelivered
- **THEN** no second notification is created

#### Scenario: A price drop after a restart is still detected

- **WHEN** a listing was seen at 2,000,000, team-notification restarts, and the price drops
  to 500,000
- **THEN** subscribed buyers receive the price-drop notification

### Requirement: Chat notifications name the sender

A chat notification SHALL name the sender: the shop display name when the sender is the
thread's seller (from team-domain storefronts), otherwise the user's display name from
team-identity. When the lookup fails or returns nothing, the title SHALL fall back to a
neutral label and the notification SHALL still be created.

#### Scenario: A seller reply names the shop

- **WHEN** a seller whose shop display name is "Nhà Sách An Nhiên" replies in a thread
- **THEN** the buyer's chat notification title contains "Nhà Sách An Nhiên"

#### Scenario: Name lookup failure still notifies

- **WHEN** the name lookup fails
- **THEN** the chat notification is created with the fallback label

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
