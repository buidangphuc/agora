# platform-featurestore

**Status: batch job** (`python -m featurestore materialize`), introduced by the OpenSpec change
`featurestore-materialization` (AI-first 4/7). It is not a server: no port, no proto, no events, no database.
It reads the warehouse Parquet exports and writes an offline snapshot plus an online copy in Redis.
Serving features to team-ai is a later change (`recs-serving-safeguards`).

## Contract

**Inputs** (read-only, `FEATURESTORE_INPUT_DIR`, written by team-analytics): `tracking_events_resolved.parquet`
(`tracking_events` columns plus `user_key`), `engagement_facts.parquet`, `order_facts.parquet` (with `buyer_id`). Optional: `listing_sellers.parquet` (`listing_id`,
`seller_id`, `updated_at`, `category_id`, `price`; the table of `ListingChanged` snapshots). Without it the attribute views
are empty and the job warns; with it but without `category_id` or `price` the job exits 2 naming the column.

**Registry** (`registry/features.yaml`, SQL in `registry/sql/`, hashes in `registry/features.lock`):

| View | Entity | Features |
| --- | --- | --- |
| `user_activity@v2` | `user_key` | `views_7d`, `clicks_7d`, `add_to_cart_7d`, `favorites_current`, `follows_current`, `paid_orders_30d` |
| `item_popularity@v1` | `listing_id` | `views_7d`, `clicks_7d`, `add_to_cart_7d`, `favorites_current`, `review_count`, `avg_rating`, `ctr_7d` |
| `item_attributes@v1` | `listing_id` | `seller_id`, `category_id` (strings), `price` (int, minor units); null when unknown |
| `user_preferences@v1` | `user_key` | `preferred_categories`: up to three category ids, comma-joined |

`item_attributes@v1` is the latest recorded change of each listing with `updated_at <= AS_OF` (a listing edited later is
absent from that snapshot). `user_preferences@v1` ranks categories by the user's views (1), clicks (2) and add-to-carts (5) on
listings with a known category in `(AS_OF - 30d, AS_OF]`, weight descending then name; users with none have no row.
Feature types are `int`, `float` and `string`.

`paid_orders_30d` is the number of distinct paid orders (`order_facts.status = 'PAID'`) whose `buyer_id` is the user,
with `occurred_at` in `(AS_OF - 30d, AS_OF]`. Order lines without a `buyer_id` (rows ingested before the column
existed) count for nobody; a buyer with orders but no events still gets a row. Refunds and cancellations do not lower
it (order_facts only holds PAID facts). `user_activity@v1` was retired by `order-facts-buyer`; the job exits 2 if
`order_facts.parquet` has no `buyer_id` column (upgrade team-analytics first, wait one export cycle).

**Point in time.** Everything is computed as of `AS_OF` (RFC 3339 with offset; empty or unset means now, UTC). The SQL
only sees the views `events` and `facts` (`ingested_at <= AS_OF`) and `orders` (`occurred_at <= AS_OF`), created by
the job. 7-day windows are `(AS_OF - 7d, AS_OF]`. Current favourites/follows use the latest fact per pair
(`occurred_at`, then `event_id`).

**Offline output.** `<offline dir>/<view>/v<ver>/as_of=<YYYYMMDDTHHMMSSZ>.parquet` (entity column plus feature columns)
and `<offline dir>/runs/<YYYYMMDDTHHMMSSZ>/manifest.json` (`as_of`, `materialized_at`, `input_watermark`, per-view
`rows`/`definition_sha256`/`snapshot`, input files with size and mtime). Earlier runs are kept.

**Online output.** `fs:<view>:v<ver>:<entity_id>` = flat JSON `{feature: value}` with TTL; then `fs:<view>:current`
(the version) and `fs:<view>:meta` (JSON `as_of`, `materialized_at`, `input_watermark`), written last. Timestamps are
RFC 3339 UTC ending in `Z`. The old `fs:u:` / `fs:i:` keys are gone.

**Parity.** After writing, a deterministic sample per view is read back from Redis and compared with the snapshot
(`isclose` 1e-9, equality otherwise). `python -m featurestore parity` repeats this on the latest manifest.

| Command | Does |
| --- | --- |
| `materialize` | compute, write offline + online, parity gate |
| `parity` | compare Redis with the latest run's snapshots |
| `dataset` | build governed datasets (`als_interactions@v1`, `rank_training@v1`) as of `AS_OF` into `<offline>/datasets/<name>/v<n>/as_of=<stamp>.parquet` + `.manifest.json`; window `DATASET_WINDOW_DAYS` (default 30) |
| `lock` | regenerate `registry/features.lock` after a deliberate definition change |

Exit codes: 0 ok, 2 config/missing input, 3 parity mismatch (`parity mismatch view=.. entity=.. feature=..
online=.. offline=..`), 4 registry drift (a definition changed without a version bump, or a view missing from the lock).

**Ranking dataset.** `rank_training@v1` has one row per (`impression_id`, `listing_id`) of an `impression` event in the
window: `user_key`, `impression_id`, `listing_id`, `position`, `label`, `occurred_at`. `label` is 2 when an `add_to_cart`
with the same `impression_id` and `listing_id` happened at or after the impression, else 1 for a `click`, else 0. Registry
datasets may declare `columns:` (name to `string|int|float|timestamp`); without it the `als_interactions` columns apply.
platform-recsys' GBDT trainer reads it (change `recsys-gbdt-trainer`).

## Configuration

| Variable | Default |
| --- | --- |
| `FEATURESTORE_INPUT_DIR` | `/data` |
| `FEATURESTORE_OFFLINE_DIR` | `/features` |
| `FEATURESTORE_REDIS_URL` | required |
| `FEATURESTORE_ONLINE_TTL_SECONDS` | `172800` |
| `FEATURESTORE_PARITY_SAMPLE` | `200` |
| `AS_OF` | now (UTC) |

## Run locally

```bash
docker build -t platform-featurestore .        # job image, entrypoint python -m featurestore
docker build --target test -t featurestore-test . && docker run --rm featurestore-test   # unit tests
docker run --rm -v analytics_data:/data:ro -v featurestore_data:/features \
  -e FEATURESTORE_REDIS_URL=redis://host:6379/2 platform-featurestore materialize
```

The compose `featurestore-job` service (profile `featurestore`) is owned by the root compose change.

## Build, test and lint

`make test` runs pytest over pyarrow fixture Parquet files with fakeredis; `make lint` runs `ruff check .`
(`pip install -r requirements.txt` first). There is no CI workflow for this repo.

## Gotchas

- Features are at most one export cycle behind the warehouse (`PARQUET_EXPORT_INTERVAL_SECONDS`).
- Users without events or facts before `AS_OF` have no row at all.
- `user_key` equals the user id for logged-in users and `anon:<id>` otherwise; engagement facts join on `user_id`.

## Links

Root `AGENTS.md`; change `openspec/changes/featurestore-materialization`; ADR-0015 (platform-core, change task 1.3).
