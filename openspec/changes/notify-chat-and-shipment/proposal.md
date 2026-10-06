## Why

The post-purchase journey e2e (now testing the real system) has two strict-xfail scenarios
for missing notifications:

1. A seller's chat reply creates no notification for the buyer: team-chat publishes
   `ChatMessage` to `chat.events` only for the gateway's live push, and team-notification
   consumes only `listing.events`.
2. Shipping an order creates no notification: team-order emits only `OrderPaidEvent`, and
   nothing turns a shipment into a notification.

`NOTIFICATION_TYPE_CHAT` and `NOTIFICATION_TYPE_ORDER` exist in the contract but nothing
creates them.

## What Changes

- platform-core: an additive `OrderShipped` event message in the order proto. If the chat
  event lacks the thread participants needed to find the recipient, an additive field or
  message for that.
- team-order: write `OrderShipped` to the outbox in the same transaction as `CreateShipment`.
- team-chat: make sure the published message event carries what a consumer needs to find
  the recipient (thread participants, or the recipient id).
- team-notification: consume `chat.events` and `order.events`, create the CHAT and ORDER
  notifications for the right user, dedupe by event id, honour notification preferences.
- platform-e2e: remove the strict xfail markers once both scenarios pass.

Repos: platform-core, team-order, team-chat, team-notification, platform-e2e (and compose
for the new consumer topics).

## Capabilities

### New Capabilities
- `buyer-notifications`: chat and shipment notifications.

## Non-goals

- No push, e-mail or SMS delivery; in-app notifications only.
- No notifications for other order transitions (cancel, refund, delivered) in this change.
- No change to the live chat push (SSE) path.

## Impact

- New consumer groups in team-notification for `chat.events` and `order.events`.
- `order.events` gains a second event type; team-analytics already filters by type.
