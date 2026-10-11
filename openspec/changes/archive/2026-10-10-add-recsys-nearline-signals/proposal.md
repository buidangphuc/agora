## Why

Batch ALS models retrain on a nightly or weekly schedule. When a user browses listings in an active session, their recent views, cart additions, and category preferences must immediately influence recommendations without waiting for the next offline batch.

Following **P2-T4**, this change introduces a nearline streaming signal layer in `platform-recsys/recsys/nearline/` that consumes `analytics.events` and maintains real-time user session signals and item co-occurrence in Redis with bounded TTLs.

## What Changes

- **platform-recsys** (`recsys/nearline/`):
  - `NearlineSignalAggregator`: Consumes tracking events (`view`, `click`, `add_to_cart`) and updates real-time user activity windows.
  - `NearlineSignalStore`: Redis-backed reader providing:
    - `get_recent_interacted_items(user_or_anon_id, limit)`
    - `get_category_affinities(user_or_anon_id)`
    - `get_realtime_co_viewed_items(item_id, limit)`
  - Unit tests in `platform-recsys/tests/test_nearline.py`.

## Non-goals

- No heavy stateful stream processing clusters (Flink/Spark Streaming) — a lightweight Redis-buffered aggregator handles our throughput with sub-second latency and zero extra operational cost.
