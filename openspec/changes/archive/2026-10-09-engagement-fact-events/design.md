## Context

See proposal.md for the motivation. The current agora code (2026-10-09):
- **team-engagement** has Postgres repositories (`internal/repository`), a Kafka consumer (`internal/consumer`, for
  listing and order projections), no producer and no outbox. Favourites and follows already treat adding an existing row
  and removing a missing one as idempotent no-ops. `CreateReview` already checks the buyer's delivered order.
- **team-order** is the reference outbox. It has:
  - an `outbox` table with an ordered `seq`;
  - `internal/repository/outbox_pg.go`, where the outbox row is written in the business transaction;
  - `internal/events/relayer.go`, which polls rows in `seq` order, publishes them, then marks them published;
  - `ORDER_OUTBOX_RELAY_INTERVAL`.
- **team-analytics** consumes `analytics.events` and `order.events` with franz-go, writes DuckDB through an idempotent
  anti-join, and owns the DLQ pattern in its consumers.
- **Topics** come from the root compose `redpanda-init` (`rpk topic create ...`) and the gitops Redpanda job
  (`platform-gitops/.../redpanda.yaml`).

## Goals / Non-Goals

**Goals:**
- Server-truth engagement facts that cannot be lost when Kafka is down, and that never duplicate a no-op.
- A warehouse table plus current-state views that the feature store can read.

**Non-Goals:** back-fill, Q&A, collections, review text, and consumption by recsys.

## Decisions

### D1. Copy the team-order outbox shape
- Migration `0010_engagement_outbox`: `outbox(seq BIGSERIAL PRIMARY KEY, event_id UUID, type TEXT, key TEXT,
  payload BYTEA, principal_id TEXT, principal_type TEXT, request_id TEXT, occurred_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ NULL)`, with a partial index on unpublished rows.
- The five service methods run their state change and the `InsertOutbox` call in one `pgx` transaction. A no-op skips
  the insert.
  - The repositories already tell a no-op apart: `ON CONFLICT DO NOTHING` affects zero rows, and a delete affects zero
    rows.
- The relayer is a copy of team-order's: it polls, publishes in `seq` order, then marks rows published. It is gated by
  `KAFKA_ENABLED`, and its interval is `ENGAGEMENT_OUTBOX_RELAY_INTERVAL`.
- Alternative considered: publish directly after commit. Rejected, because a crash between commit and publish loses the
  fact, which is exactly the gap this change closes.

### D2. Payloads (`platform/engagement/v1/events.proto`)
- `FavoriteAdded{user_id, listing_id}`
- `FavoriteRemoved{user_id, listing_id}`
- `SellerFollowed{user_id, seller_id}`
- `SellerUnfollowed{user_id, seller_id}`
- `ReviewCreated{review_id, user_id, listing_id, seller_id, rating}`

The seller id on `ReviewCreated` comes from engagement's own `seller_listings` projection. When it is unknown, the field
is left empty.

The new file is vendored into team-engagement and team-analytics.

### D3. Analytics sink
- A new consumer runs on the same process pattern as the order consumer. Its group is `team-analytics.engagement` and
  its topic is `ENGAGEMENT_EVENTS_TOPIC`.
- A `switch` on the envelope `type` maps each payload to an `engagement_facts` row with `fact` set to `favorite_added`,
  `favorite_removed`, `seller_followed`, `seller_unfollowed` or `review_created`.
- The insert is idempotent on `event_id`, using the existing anti-join helper.
- A record that fails to decode, or has an unknown type, goes to `ENGAGEMENT_DLQ_TOPIC`, and the consumer commits past
  it.
- `ingested_at` is set at write time.
- The views:
  - `favorites_current`: `arg_max(fact, occurred_at)` per (user_id, listing_id) where `fact` is a favourite fact,
    keeping the pairs whose latest fact is `favorite_added`;
  - `follows_current`: the same over the follow facts.
  - Ties on `occurred_at` resolve by `event_id` order, which is acceptable because they come from one user's sequential
    calls.

### D4. Topics
Add `engagement.events` and `engagement.events.analytics.dlq` to the `redpanda-init` list and to the gitops Redpanda
topics job. Add the env vars to the gitops values for team-engagement and team-analytics.

## Risks / Trade-offs

- **[A transaction now spans two tables in hot paths (favourite toggling)]** Mitigation: one small insert. The relayer
  is off the request path.
- **[Outbox growth]** Mitigation: published rows older than 7 days are deleted by the relayer each cycle, matching
  team-order.
- **[The Kafka-down scenario stops Redpanda, which every service uses]** Mitigation: it runs in the destructive lane and
  waits for consumers to recover. The proven `wait_settled` helpers apply.

## Migration Plan

Deploy in this order: proto and vendoring first, then team-analytics (the consumer tolerates an empty topic), then
team-engagement. Rollback reverts the images. The outbox table is harmless; its down migration drops it.
