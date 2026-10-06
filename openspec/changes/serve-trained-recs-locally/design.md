## Context

- team-analytics `duckdb.Writer.ExportParquet(ctx, dst)` exists (`COPY ... TO ... (FORMAT
  PARQUET)`) but has no caller. DuckDB holds an exclusive file lock, so no other
  process may read the database file. An export from inside the service is the only
  safe read path.
- platform-recsys reads `WAREHOUSE_PARQUET_PATH` (duckdb driver). It writes
  `item_als_vectors` and `user_als_vectors` to Qdrant, with point id
  `uuid5(6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00, source_id)` and the source id in the
  payload, plus Redis lists `recs:v1:user:{user_key}` and `recs:v1:popular`, where
  `user_key` = principal id.
- team-ai's ladder reads the Redis user list first, then Qdrant similarity, then the
  Redis popular list.

## Decisions

1. **Export inside team-analytics**: a ticker goroutine runs every
   `PARQUET_EXPORT_INTERVAL_SECONDS` (0 = off, the default) and calls `ExportParquet`
   to `<path>.tmp`, then renames it to `PARQUET_EXPORT_PATH`, so a reader never sees a
   partial file. It also exports once at startup when enabled. Export failures are
   logged and never stop the consumer.
2. **Volume ownership**: the image copies an empty, nonroot-owned `/data`, so a fresh
   named volume inherits that ownership. The existing root-owned volume is fixed once
   at rollout (chown, this project's volume only).
3. **Training is a compose job** (`profiles: [jobs]`, `docker compose run --rm
   platform-recsys`). It mounts the analytics volume read-only and uses the
   `qdrant`/`redis` service names. `ALS_RANK` stays 64 = team-ai `RECS_VECTOR_DIM`.
4. **Point-id mapping in team-ai** duplicates the producer's namespace constant. A
   unit test pins it to the value platform-recsys uses, so a drift fails tests.

## Risks / Trade-offs

- Small, synthetic interaction data gives weak models. The goal is a working path,
  not quality.
- Every export rewrites the whole table. That is fine at local volumes.
