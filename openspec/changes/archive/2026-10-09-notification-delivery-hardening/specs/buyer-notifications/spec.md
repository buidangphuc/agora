## ADDED Requirements

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
