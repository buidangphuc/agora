## Why

The home "Gợi ý cho bạn" row never renders on the local stack, and the buyer funnel
journey keeps a strict xfail for it. The pipeline exists in parts, but no stage
feeds the next:

1. team-analytics stores tracking events in DuckDB under `/tmp`, because the
   `/data` volume is root-owned and the image runs as nonroot. A restart loses the
   events, and nothing ever runs `ExportParquet`, so platform-recsys has no input.
2. platform-recsys (Spark ALS) is never run against the stack, so Qdrant and Redis
   hold no recommendations.
3. team-ai runs with `RECS_ENABLED=false`. The image lacks `qdrant-client`. Its
   Qdrant reader looks up raw listing ids, while the producer stores uuid5 point
   ids, and it returns the point id instead of the listing id.

## What Changes

- team-analytics: keep DuckDB on the `/data` volume (the image prepares a
  nonroot-owned `/data`). Add an optional periodic Parquet export
  (`PARQUET_EXPORT_PATH`, `PARQUET_EXPORT_INTERVAL_SECONDS`), written atomically.
- compose: a `platform-recsys` job service (profile `jobs`) that reads the
  exported Parquet from the analytics volume and writes to the stack's Qdrant and
  Redis. team-ai gets `RECS_ENABLED=true` and `RECS_BACKEND=qdrant`.
- team-ai: add the `qdrant-client` dependency. Map listing ids to the producer's
  uuid5 point ids, pinned by a test. Return the payload `listing_id`.
- platform-e2e: remove the strict xfail on the home recommendations scenario.

Repos: team-analytics, team-ai, platform-e2e, compose.

## Capabilities

### Modified Capabilities
- `recommendations`: trained recommendations served on the local stack.

## Non-goals

- A scheduler for the training job (run by hand or by a gitops CronJob later).
- gitops/helm changes.
- Two-Tower, GBDT ranking or any model change.

## Impact

- The analytics data location moves from `/tmp` to the volume. The events
  collected so far are copied over once, by hand, during rollout.
- team-ai image grows by the qdrant-client wheel.
