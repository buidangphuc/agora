## Context

The previous change left `category_id`, `price`, `preferred_categories` at 0 in the towers because no view carries them
(`wire-two-tower-batch-pipeline`, Follow-ups). team-analytics' `ListingChanged` consumer already stores `listing_sellers`
(listing -> seller); `Listing.category_id` and `Listing.price` are in the same event, so no proto change is needed.

## Decisions

### D1. The source is the existing listing consumer (analytics side)
`listing_sellers` gains `category_id VARCHAR` and `price BIGINT` through `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` (the
same migration style as `tracking_events`). The upsert keeps its guard (`excluded.updated_at >= stored`), so an
out-of-order event cannot regress a row and a replay of the same event refreshes it. The table keeps one row per listing:
the latest event wins, and a DELETED event keeps its row (history stays attributable). The table name stays (it is read
by the seller funnel); the export writes it as `listing_sellers.parquet`. Chosen over a second `listing_attributes` table:
one consumer, one transaction, no second source of truth for the same event.

Backfill: rows already stored have NULL category and price. The listing consumer group default changes
(`team-analytics-listing-sellers` -> `team-analytics-listing-attrs`), so it reads `listing.events` from the earliest offset
once and refreshes every listing. A deployment that sets `KAFKA_LISTING_CONSUMER_GROUP` explicitly must change it too.

### D2. Point in time
The table keeps only the latest version of a listing, so the featurestore filters `updated_at <= AS_OF` when it loads
`listings` (the same place events are filtered by `ingested_at`). A listing edited after `AS_OF` is therefore absent from
that snapshot instead of leaking a future value; the cost is that it is missing from snapshots older than its last edit.
Snapshots are run at "now" in production, where the two coincide.

### D3. The views
`item_attributes@v1` returns the row of `listings` as is: `seller_id`, `category_id` (string), `price` (int). Unknown is
NULL. `user_preferences@v1` joins `events` (view 1, click 2, add_to_cart 5, the weights `als_interactions` and the nearline
category affinities use) with `listings` on `listing_id`, 30 days, ranks categories per user by weight then name and keeps
three. Online values are flat JSON like every view; the list is a comma-joined string (the feature types are int, float and
string), so a category id containing a comma is skipped. avg order value is not added: nothing in the serving or training
contracts asks for it yet.

### D4. A missing listing export is not a failure; a stale one is
Existing synthetic-input runs (order features, the unit fixtures) have no `listing_sellers.parquet`. Making it mandatory
would fail them for a view they do not use, so a missing file gives empty `listings`, a warning on stderr and empty
attribute views. A file that exists but lacks `category_id` or `price` is an old exporter: exit 2 naming the column, the same
rule as `buyer_id` in `order_facts`. The registry lock gets the two new hashes (`python -m featurestore lock`).

### D5. Two-tower consumption (recsys)
`stage.resolve_inputs` also resolves `item_attributes@v1` and `user_preferences@v1` (`resolve_snapshot`, directories
`ITEM_ATTRIBUTES_DIR=/features/item_attributes/v1`, `USER_PREFERENCES_DIR=/features/user_preferences/v1`, explicit
`*_PATH`). They are optional unless `TWO_TOWER_REQUIRE_ATTRIBUTES=true` (default false, so the existing job images and
fixtures keep running); required and missing: `ConfigError` before Spark, exit 2. The catalogue becomes the union of the
popularity snapshot and the attribute snapshot, so a listing that exists but has no events yet (the real cold start) gets a
vector. `item_features` adds `category_id` and `price`; `user_features` adds `preferred_categories` (split on the comma).
The tower's fixed eight-name vocabulary cannot match real ids such as `cat-electronics`, so the vocabulary is built from the
attribute snapshot (descending frequency then name, capped by `TWO_TOWER_MAX_CATEGORIES`, default 64) and passed to
`TwoTowerModel`; without attributes the default vocabulary is used as before. `parameters.two_tower.features` gains
`attributes`, `preferences` (lineage or null) and `category_vocab` (size).

## Risks
- A listing that never changes again is refreshed only by the replay (D1): until the new consumer group has caught up,
  attributes are partial. The item vectors of those listings use 0 for the missing inputs, as before.
- The vocabulary is per run; a future user-tower query path must read it from the model metadata (not recorded as a list
  here to keep the metadata small; the size is recorded and the order is the snapshot's frequency order).
