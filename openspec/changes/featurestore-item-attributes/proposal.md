## Why

The two-tower towers take category, price and preferred categories as inputs, but the featurestore has no view that
carries them: `item_popularity@v1` and `user_activity@v2` are engagement counts only. The archived
`wire-two-tower-batch-pipeline` change therefore trains with those inputs at 0, and its cold-start scenario proves a
cold item gets a vector, not that the vector reflects what the item is. team-analytics already consumes
`listing.events` (it keeps `listing_sellers`), but only keeps the owner: the export has no category or price.

## What Changes

- **team-analytics** (additive): `listing_sellers` also keeps `category_id` and `price` from each `ListingChanged`
  snapshot (idempotent column migration, newest event wins); the Parquet export cycle also writes
  `listing_sellers.parquet` (`listing_id, seller_id, updated_at, category_id, price`). The listing consumer group
  default changes so existing listings are replayed once and backfilled.
- **platform-featurestore**: reads the new export as the point-in-time table `listings` and registers
  `item_attributes@v1` (entity `listing_id`: `seller_id`, `category_id`, `price`) and `user_preferences@v1`
  (entity `user_key`: `preferred_categories`, the top three categories of the user's views, clicks and add-to-carts
  in the 30 days before `AS_OF`). Offline snapshot, versioned online keys, parity and the lock follow the existing views.
- **platform-recsys**: the two-tower stage reads the latest `item_attributes@v1` and `user_preferences@v1` snapshots
  (category, price, preferred categories), adds attribute-only listings to the catalogue, builds the category vocabulary
  from the data, and records the snapshots and the vocabulary size in the model metadata. `TWO_TOWER_REQUIRE_ATTRIBUTES`
  makes missing snapshots a configuration error.

## Capabilities

### New Capabilities
- `item-attributes`: the listing export and the `item_attributes@v1` / `user_preferences@v1` feature views.

### Modified Capabilities
- `recommendations`: the two-tower stage consumes the attribute views.

## Impact

- team-analytics: `internal/warehouse`, `internal/consumer/listing.go`, `internal/export`, config default
  `KAFKA_LISTING_CONSUMER_GROUP`. No proto change (`Listing.category_id`/`price` already exist).
- platform-featurestore: `featurestore/inputs.py`, registry (2 views, SQL, lock), README, tests.
- platform-recsys: `recsys/two_tower/{features,stage,pipeline}.py`, config + `.env.example`, tests.
- Images to rebuild: team-analytics, platform-featurestore, platform-recsys. No new dependency.
- team-ai is untouched (the item vectors are published as before).

## Non-goals

- No change to ALS or to the retrieval path in team-ai; no user-tower query path.
- No average order value, seller or price-band features; no category taxonomy service.
- No proto change and no new topic.
