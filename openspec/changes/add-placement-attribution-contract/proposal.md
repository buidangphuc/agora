## Why

Recommendations and search require end-to-end attribution tracking. Today's `TrackingEvent` proto and gateway beacon capture general event signals (`view`, `click`, `add_to_cart`, `impression`) with page context, but lack first-class attribution metadata:
1. `placement_id`: which placement rendered the item (e.g. `home_feed`, `similar_items`, `cart_cross_sell`, `search_results`).
2. `impression_id`: a unique UUID linking an item impression directly to downstream click, cart addition, and purchase events.
3. `model_version`: the specific model artifact / pipeline version that produced the recommendation (e.g. `als_v1`, `two_tower_v2`, `popular_baseline`).

Without these fields, offline evaluations and online A/B attribution cannot attribute conversions back to specific placement slots or algorithm generations.

## What Changes

- **platform-core** (`packages/proto/platform/analytics/v1/analytics.proto`):
  - Add additive fields to `TrackingEvent`:
    - `string placement_id = 10;`
    - `string impression_id = 11;`
    - `string model_version = 12;`
  - Ensure standard buf lint and non-breaking checks pass.
- **team-gateway**:
  - Update `trackBeacon` struct and `HandleTrack` collector to parse `placementId`, `impressionId`, and `modelVersion` from beacon payloads (JSON object and array).
  - Forward these fields into `analyticsv1.TrackingEvent` when publishing to Kafka `analytics.events`.
  - Add unit tests verifying beacon ingestion and attribution field preservation.
- **team-analytics**:
  - Update `warehouse.TrackingRecord` and `RecordFromEnvelope` to extract and preserve attribution fields into the warehouse schema.
- **team-frontend**:
  - Update `src/lib/track.ts` to accept optional `placementId`, `impressionId`, `modelVersion`.

## Non-goals

- No change to event envelope security or principal propagation — identity continues to travel securely in `EventEnvelope.principal`.
- No backward-incompatible proto field numbering changes.
