# Tasks

> **Reality check 2026-09-20** — every task below was genuinely done: `pipeline.py:88-100` does
> call `train_and_index_two_tower` and does load vectors into Qdrant. The defect is not a false
> tick, it is that **no task or scenario constrained what the vectors contain** — and they are
> all zero. Added as new tasks rather than un-ticking work that was performed.

> **Merged 2026-10-09:** add-two-tower-retrieval is retired into this change. Its towers and model
> (`recsys/two_tower/{user_tower,item_tower,model}.py`, `tests/test_two_tower.py`) are done. Its projection and top-K
> requirements are in this change's spec delta. Its cold-start requirement is covered by "Cold-start item receives a
> vector that ALS cannot produce".

## 1. Code — platform-recsys
- [x] Add Two-Tower settings to `recsys/config.py` (`ENABLE_TWO_TOWER`, `QDRANT_TWO_TOWER_COLLECTION`, `TWO_TOWER_DIM`).
- [x] Keep `.env.example` in sync with `_FIELDS`.
- [x] Implement `load_two_tower_vectors` in `recsys/load/qdrant.py`.
- [x] Wire Two-Tower training and Qdrant loading into `recsys/pipeline.py:run()`.
- [x] Write execution-proof tests in `tests/test_two_tower_pipeline.py`.

## 2. Code — the wire carries nothing (found 2026-09-20)
- [ ] Feed real item features into the tower (item_popularity@v1 / user_activity@v2 snapshots, `two_tower/features.py`,
      `stage.py`; ConfigError exit 2 without them).
- [ ] Give the tower an actual training step (`two_tower/train.py`, in-batch softmax; loss in the summary and metadata).
- [ ] Refuse zero / non-finite vectors, fail the stage when none remain (`two_tower/pipeline.py`).
- [ ] Generation-named collection written by `publish_generation` before the switch, retired by retention
      (`load/qdrant.py`, `publish.py`).

## 3. Verification
- [x] Run `pytest -v tests/test_two_tower_pipeline.py tests/test_env_drift.py`.
- [ ] Strengthen "Cold-start item receives a vector that ALS cannot produce": non-zero and distinct
      (`test_items_with_different_categories_embed_differently_and_non_zero`, `tests/test_pipeline_two_tower.py`).
- [ ] Tests for the merged tower requirements: `test_towers_projection_and_normalization`,
      `test_top_k_is_ranked_by_similarity_and_bounded`.
- [x] `openspec validate wire-two-tower-batch-pipeline --strict`.
- [ ] e2e (`platform-e2e` `features/recommendations/mlr_two_tower.feature`): the real job image with feature snapshot
      fixtures. Needs the rebuilt image.
- [ ] Follow-up (featurestore): item/user attribute view with category, price, preferred categories.
