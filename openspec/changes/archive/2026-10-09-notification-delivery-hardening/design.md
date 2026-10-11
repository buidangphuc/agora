## Context

team-order, team-domain, team-payment and team-identity already use a transactional outbox
with a relayer whose claim query keeps creation order (CTE + outer ORDER BY). The
team-notification consumers take injectable `Deduper`, `PriceStateStore` and
`StockStateStore` interfaces with in-memory defaults (`WithDeduper`,
`WithPriceStateStore`, `WithStockStateStore`). team-domain exposes `BatchGetStorefronts`
(`listing.read`).

## Decisions

1. **Chat outbox mirrors team-order's**: same table shape, the `OUTBOX_*` env names, the
   ordered claim query and its Postgres ordering test. The direct `PublishMessageSent` call
   is replaced by an outbox write in the message transaction.
2. **Durable consumer state** implements the existing interfaces in Postgres
   (`processed_events(consumer, event_id)` primary key; `listing_last_seen(listing_id,
   price, stock)`), wired in main when the database is enabled. In-memory stays the
   default for tests.
3. **Sender names are resolved by team-notification** (async, off the send path), never
   by reading another service's DB: the thread's seller via team-domain
   `BatchGetStorefronts`, anyone else via a new identity RPC
   `GetPublicProfiles(user_ids) → {user_id, display_name}` gated to service principals.
   The display name is the username unless a display name exists. Short timeout; any
   failure falls back to "Người dùng".
4. **Service-to-service auth** uses the platform's existing service-principal convention for
   internal gRPC calls (look at how team-order calls team-domain).

## Risks / Trade-offs

- Two extra gRPC calls per chat notification; bounded by a timeout, and failures never
  block the notification.

## Open Questions

None blocking.
