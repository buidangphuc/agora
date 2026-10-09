## Context

See proposal.md for the motivation. Current code (2026-10-09):

**platform-featurestore** (after `featurestore-materialization`):
- `python -m featurestore {materialize,parity,lock}`.
- `inputs.connect` loads AS_OF-filtered temp tables `events`/`facts`/`orders` from the analytics Parquet exports, then
  disables external access.
- The registry is `registry/features.yaml` plus `sql/*.sql`, guarded by `features.lock`.

**platform-recsys:**
- `pipeline.run` reads `warehouse.read_tracking_events` (raw `tracking_events.parquet`, a rolling wall-clock window).
- `interactions.py` builds (user, item, weight) with `settings.event_weights` and `weights.choose_user_key`
  (`principal_id`, else `anonymous_id`).
- `evaluate_generation` splits on event time.
- `ModelRegistry.register_model` stores `ModelMetadata` JSON in Redis, with `parameters: dict`.

**e2e:** `tests/e2e/flows/recsys_job_flow.py` and `recsys_job_driver.py` run the recsys image with a synthetic
tracking-events Parquet fixture.

## Goals / Non-Goals

**Goals:**
- One governed dataset definition, point-in-time and reproducible, consumed by ALS with lineage.

**Non-Goals:**
- Two-tower or GBDT.
- Generation publish.
- An object-storage dataset location for deployed environments.

## Decisions

### D1. The dataset is a registry entry beside the views
- `registry/features.yaml` gains `datasets: [{name: als_interactions, version: 1, sql: sql/als_interactions_v1.sql}]`.
  It is hashed into `features.lock` like the views.
- `python -m featurestore dataset` runs every registered dataset on the same locked connection as `materialize`.
- Alternative considered: a separate `datasets.yaml`. Rejected, because one lock and one review surface is simpler.

### D2. Weight SQL
- The event weights are a `VALUES` table inside the SQL file, with the numbers moved verbatim from recsys
  `DEFAULT_EVENT_WEIGHTS`, so the weights version with the definition.
- Favourite state is `arg_max(fact, occurred_at)` per pair over the favourite facts, current when it is
  `favorite_added`.
- The review adjustment uses the latest review per pair.
- The result groups by (`user_key`, `listing_id`) and keeps `weight > 0`.
- `interactions` counts the tracking events, plus 1 for a current favourite, plus 1 for a review.
- `last_occurred_at` is the max over all contributing rows.
- Engagement facts join on `user_id` = `user_key`. This holds because facts come only from logged-in users.

### D3. Output and manifest
- Output path: `<offline>/datasets/<name>/v<version>/as_of=<stamp>.parquet`.
- `manifest.json` sits beside it as `as_of=<stamp>.manifest.json`, so each dataset file has its own manifest.
- The file SHA-256 is computed from the bytes after the atomic rename.

### D4. recsys reads the dataset
- New `recsys/dataset.py`:
  - `resolve_dataset(settings)` returns `DATASET_PATH`, or the lexically latest `as_of=*.parquet` under
    `DATASET_DIR` together with its manifest.
  - It raises `ConfigError("no governed dataset under DATASET_DIR=…")` when there is none. `__main__` exits 2 on that
    error.
- `pipeline.run` uses `spark.read.parquet(dataset)`. The columns are `user_key` (as the user id), `listing_id` and
  `weight`.
- ALS and the evaluation split use `last_occurred_at` (temporal holdout per user, as before).
- `interactions.py`'s event-weight path is deleted for ALS. `weights.choose_user_key` stays only for nearline and
  two-tower, which are unchanged.
- `ModelMetadata.parameters["dataset"]` is `{name, version, as_of, sha256}`, copied from the manifest.

### D5. Compose and e2e fixtures
- The recsys job service mounts `featurestore_data:/features:ro` and sets
  `DATASET_DIR=/features/datasets/als_interactions/v1`.
- The featurestore job gains a `dataset` command, run with `command: ["dataset"]`, or chained as
  `materialize && dataset` through an entrypoint flag.
- e2e:
  - The existing recsys scenarios switch to a dataset fixture: the driver writes a small `als_interactions` Parquet
    file and a manifest into a temp `DATASET_DIR`.
  - New scenarios run the real featurestore image, then the real recsys image, on the stack.

## Risks / Trade-offs

- **The ALS quality may shift,** because stitching and favourites change the interaction graph. Mitigation: the
  promotion gate (holdout eval vs champion) still decides publication, so a worse model is not promoted.
- **The export interval (300 s) makes the new e2e slow.** Mitigation: as in change 4, the scenarios share one batch of
  activity and wait for one export cycle.

## Migration Plan

Deploy the featurestore image with `dataset`, then recsys. Rollback is a revert of recsys. Datasets on disk are
harmless.
