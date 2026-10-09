## Why

This is AI-first change 3 of 7. The feature store and the recommenders (changes 4–7) need the strongest preference
signals the platform has: favourites, follows and review ratings. Today the only record of them is client tracking:
- It is best-effort (`sendBeacon`).
- It can be blocked by the browser.
- It never records a removal.
- It has no rating.

team-engagement owns the truth but publishes nothing. Its only Kafka use is a consumer. So a model can only learn
"favourited" from a beacon that may never have arrived, and never learns "unfavourited".

## What Changes

**platform-core (proto, additive).** A new `platform/engagement/v1/events.proto` defines five payloads:
- `FavoriteAdded`
- `FavoriteRemoved`
- `SellerFollowed`
- `SellerUnfollowed`
- `ReviewCreated` (listing, seller, rating; no review text)

They ride the standard `EventEnvelope` on a new topic, `engagement.events`.

**team-engagement.**
- A transactional outbox (migration 0010) records each fact in the same transaction as the state change:
  `AddFavorite`, `RemoveFavorite`, `FollowSeller`, `UnfollowSeller`, `CreateReview`.
- A relayer publishes the outbox in order to `engagement.events`, keyed by the aggregate (listing id or seller id).
- An idempotent no-op writes no fact. For example, favouriting an already-favourited listing.

**team-analytics.** A consumer of `engagement.events` writes `engagement_facts`:
- columns `event_id`, `fact`, `user_id`, `listing_id`, `seller_id`, `rating`, `occurred_at`, `ingested_at`;
- idempotent on `event_id`;
- undecodable records go to the DLQ `engagement.events.analytics.dlq`.

It also adds two views:
- `favorites_current`: the latest add or remove per (user, listing).
- `follows_current`: the latest follow or unfollow per (user, seller).

**Topics.** Both topics are created by the local `redpanda-init` and by the gitops Redpanda topic job.

**platform-e2e.** Scenarios through the edge, asserting `engagement.events` and the warehouse.

Repos touched: platform-core, team-engagement, team-analytics, platform-gitops, root compose, platform-e2e.
**Proto change: additive** (new file).

## Capabilities

### New Capabilities
- `engagement-facts`: which engagement state changes become facts, how they are published, and how the warehouse
  stores them and derives the current state.

### Modified Capabilities
- None. Engagement RPC behaviour is unchanged.

## Non-goals

- Q&A, collections, check-ins, disputes and recently-viewed. They are not preference signals the recommenders use yet.
- Review text, which is free text and personal data. Only the rating travels.
- Using the facts in features or training. That is changes 4–6.
- Back-filling facts for state that existed before this change. The views start from the first published fact. A
  back-fill job is a follow-up if the trainer needs history.

## Impact

- team-engagement gains a Kafka producer and these settings:
  - `KAFKA_ENABLED` for the producer
  - `ENGAGEMENT_EVENTS_TOPIC`, default `engagement.events`
  - `ENGAGEMENT_OUTBOX_RELAY_INTERVAL`, default `500ms`

  With Kafka disabled, the outbox still records rows; they publish when Kafka is enabled.
- team-analytics gains `ENGAGEMENT_EVENTS_TOPIC`, `ENGAGEMENT_DLQ_TOPIC` and a consumer group,
  `team-analytics.engagement`.
- Warehouse: a new table and two views, created idempotently.
