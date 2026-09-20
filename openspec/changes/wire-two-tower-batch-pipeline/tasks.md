# Tasks

> **Reality check 2026-09-20** — every task below was genuinely done: `pipeline.py:88-100` does
> call `train_and_index_two_tower` and does load vectors into Qdrant. The defect is not a false
> tick, it is that **no task or scenario constrained what the vectors contain** — and they are
> all zero. Added as new tasks rather than un-ticking work that was performed.

## 1. Code — platform-recsys
- [x] Add Two-Tower settings to `recsys/config.py` (`ENABLE_TWO_TOWER`, `QDRANT_TWO_TOWER_COLLECTION`, `TWO_TOWER_DIM`).
- [x] Keep `.env.example` in sync with `_FIELDS`.
- [x] Implement `load_two_tower_vectors` in `recsys/load/qdrant.py`.
- [x] Wire Two-Tower training and Qdrant loading into `recsys/pipeline.py:run()`.
- [x] Write execution-proof tests in `tests/test_two_tower_pipeline.py`.

## 2. Code — the wire carries nothing (found 2026-09-20)
- [ ] Feed real item features into the tower.
      `pipeline.py:89` builds the catalog as `[{"listing_id": lid} for lid in item_ids]`, but
      `ItemTower._extract_input_vector` (`two_tower/item_tower.py:42-55`) reads `category_id`,
      `price`, `historical_ctr`, `popularity_score` — none present. Input is all zeros;
      `project()` computes `bias[j] + 0` with `bias = [0.0]*dim`; ReLU → 0; the `norm > 1e-9`
      guard then skips normalization. **Every listing gets the same zero vector**, upserted into
      a Cosine collection. Depends on a real feature source (see the measurement/feature-store
      track).
- [ ] Give the tower an actual training step, or rename it.
      `train_and_index_two_tower` performs no gradient step. Weights are
      `random.Random(seed + 100).uniform(-0.1, 0.1)` (`item_tower.py:34-39`) and are never
      updated. Either train it or stop calling it training.
- [ ] Fail the run (or refuse the upsert) when a produced vector is all zeros — a degenerate
      embedding must not reach Qdrant silently.

## 3. Verification
- [x] Run `pytest -v tests/test_two_tower_pipeline.py tests/test_env_drift.py`.
- [ ] Strengthen the scenario "Cold-start item receives a vector that ALS cannot produce": a
      zero vector satisfies it as written. Assert the vector is **non-zero and distinct**
      between two items of different categories.
- [x] `openspec validate wire-two-tower-batch-pipeline --strict`.
