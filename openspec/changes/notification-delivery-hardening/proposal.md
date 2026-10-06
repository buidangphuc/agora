## Why

`notify-chat-and-shipment` shipped with three known limitations:

1. team-chat publishes `chat.events` best-effort (no outbox), so a Kafka hiccup loses the
   event and the recipient's notification.
2. team-notification keeps its dedupe ledger and the listing consumer's last-seen
   price/stock in memory, so a restart can notify twice or miss a price drop.
3. Chat notifications name the sender as "User xxxxxx".

## What Changes

- team-chat: an outbox (`chat_outbox_events`) written in the message transaction and a
  relayer to `chat.events`, reusing the ordered claim pattern of team-order/team-domain.
- team-notification: Postgres tables for processed event ids and last-seen listing
  price/stock, used by the listing, chat and order consumers.
- team-notification resolves the chat sender's display name: shop display name via
  team-domain `BatchGetStorefronts` for the thread's seller, otherwise the user's display
  name via a new additive team-identity RPC; falls back to a neutral label on failure.
- platform-core: an additive identity RPC returning public display names for a batch of
  user ids (service-scoped).

Repos: platform-core, team-chat, team-notification, team-identity, platform-e2e, compose.

## Capabilities

### Modified Capabilities
- `buyer-notifications`: durable delivery and named chat senders.

## Non-goals

- gitops/helm env sync (handled as a separate step after this change merges).
- No change to the notification UI.

## Impact

- New migrations in team-chat and team-notification.
- team-notification gains gRPC clients for team-domain and team-identity (service
  principal).
