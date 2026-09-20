# Tasks

## 1. Code — platform-recsys
- [x] Add Two-Tower settings to `recsys/config.py` (`ENABLE_TWO_TOWER`, `QDRANT_TWO_TOWER_COLLECTION`, `TWO_TOWER_DIM`).
- [x] Keep `.env.example` in sync with `_FIELDS`.
- [x] Implement `load_two_tower_vectors` in `recsys/load/qdrant.py`.
- [x] Wire Two-Tower training and Qdrant loading into `recsys/pipeline.py:run()`.
- [x] Write execution-proof tests in `tests/test_two_tower_pipeline.py`.

## 2. Verification
- [x] Run `pytest -v tests/test_two_tower_pipeline.py tests/test_env_drift.py`.
- [x] `openspec validate wire-two-tower-batch-pipeline --strict`.
