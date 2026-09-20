## Why

`add-two-tower-retrieval` (P3-T3) delivered `recsys/two_tower/` with a working
`train_and_index_two_tower`. Grep shows its only callers are `tests/test_two_tower.py` and the
package re-export — `recsys/pipeline.py` and `recsys/__main__.py` never mention it. The nightly
CronJob therefore still produces ALS factors only, and **no two-tower vector has ever reached
Qdrant**.

The consequence matters beyond tidiness: two-tower exists to give cold-start items a vector
(ALS sets `coldStartStrategy="drop"`). Until the batch entrypoint calls it, the cold-start
problem it was built to solve is untouched.

## What Changes

- **platform-recsys** (`recsys/pipeline.py`, `recsys/__main__.py`):
  - `run()` invokes `train_and_index_two_tower` over the catalog after ALS training.
  - Two-tower vectors are loaded into their own Qdrant collection, stamped with the same
    `model_version` and pruned by generation, matching the existing ALS output contract.
  - The pipeline summary reports `two_tower_items`.
  - A settings flag gates the stage so the ALS-only path stays runnable.
- **ALS is unchanged and remains the baseline** — this change adds a producer, it does not
  replace one.

## Non-goals

- No serving-side blending of the two collections (that is the placement engine's ladder).
- No removal of ALS.
- No online/incremental indexing — batch only.
