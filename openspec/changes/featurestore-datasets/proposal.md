## Why

AI-first change 5 of 7. The recsys trainer reads the raw `tracking_events` export (`platform-recsys/recsys/warehouse.py`)
and turns it into interactions with its own copy of the event weights (`EVENT_WEIGHTS_JSON`). This bypasses everything
the earlier changes built:

- Identity stitching: it keys users by `principal_id`, else `anonymous_id`, so a logged-in buyer's anonymous history is
  a different user.
- The point-in-time rule: it uses a rolling window from the job's wall clock, so training cannot be reproduced.
- The server-truth engagement facts: favourites and reviews are ignored.

A model in the registry also cannot say which data it was trained on.

## What Changes

- **platform-featurestore:**
  - `python -m featurestore dataset` builds the governed dataset `als_interactions@v1` as of `AS_OF`, from the same
    point-in-time inputs as the feature views. A row is `user_key, listing_id, weight, interactions, last_occurred_at`.
    The weight is the sum of these signals over the `DATASET_WINDOW_DAYS` before `AS_OF`:
    - the tracking event weights, which move here from recsys unchanged;
    - +3 for a current favourite;
    - +2 for a review rated 4 or 5;
    - −2 for a review rated 1 or 2.

    Pairs with a weight ≤ 0 are dropped.
  - Each build writes `<offline dir>/datasets/als_interactions/v1/as_of=<as_of>.parquet` and a manifest. The manifest
    records `as_of`, the window, the row/user/item counts, the definition hash, the input watermark and a SHA-256 of
    the dataset file.
  - The dataset definition is in the registry, under the same version and hash lock as the feature views.
- **platform-recsys:**
  - The ALS pipeline reads the governed dataset (`DATASET_PATH`, or the latest snapshot under `DATASET_DIR`) instead of
    raw events, and uses its `weight` as given. The evaluation split uses `last_occurred_at`.
  - The job refuses to start without a dataset; there is no silent fallback to raw events.
  - Every registered model records its dataset lineage in `parameters.dataset`: the name, version, `as_of` and file
    SHA-256.
  - `EVENT_WEIGHTS_JSON`, `WAREHOUSE_PARQUET_PATH` and `INTERACTION_WINDOW_DAYS` are removed from the ALS path.
- **Compose:**
  - the `featurestore-job` command can build datasets;
  - the recsys job mounts `featurestore_data` read-only and points `DATASET_DIR` at it.
- **platform-e2e:**
  - new scenarios for the dataset and lineage;
  - the existing recsys pipeline scenarios move from a raw-events fixture to a dataset fixture.

Repos touched: platform-featurestore, platform-recsys, root compose, platform-e2e. **No proto change.**

## Capabilities

### New Capabilities
- `governed-datasets`: what a governed training dataset contains, how it is built point-in-time, its manifest, and how
  recsys must consume it and record lineage.

### Modified Capabilities
- None. The `recommendations` capability's serving behaviour is unchanged. The trainer's input contract is specified
  in the new capability.

## Non-goals

- The two-tower and GBDT pipelines. They keep their current inputs and move in a follow-up once ALS is proven.
- Publishing and rollback of generations. That is `recsys-generation-publish`, change 6.
- Per-user order counts. `order_facts` still has no buyer column.

## Impact

- **New settings:**
  - featurestore: `DATASET_WINDOW_DAYS` (default 30)
  - recsys: `DATASET_DIR` (default `/features/datasets/als_interactions/v1`) and `DATASET_PATH` (an explicit file,
    which overrides `DATASET_DIR`)
- **Removed from recsys ALS:** `WAREHOUSE_PARQUET_PATH`, `INTERACTION_WINDOW_DAYS` and `EVENT_WEIGHTS_JSON`. The
  weights now live in the dataset definition.
- **BigQuery driver:** the recsys BigQuery read path for ALS is replaced by the dataset read. Deployed environments
  would materialise datasets to object storage. That is a follow-up; nothing is deployed today.
