# Tasks

## 1. Code — platform-recsys & team-ai
- [x] Implement `recsys/ranker/features.py` (feature extraction from candidates and user context).
- [x] Implement `recsys/ranker/model.py` (`GBDTRanker` scoring model).
- [x] Implement `recsys/ranker/__init__.py`.
- [x] Update `team-ai/app/modules/business/recommend/ranking.py` to use multi-feature ranking scoring.
- [x] Write unit tests in `platform-recsys/tests/test_ranker.py` demonstrating NDCG@10 gain over raw cosine.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-gbdt-ranker --strict`).
- [x] Run `pytest -v tests/test_ranker.py` in `platform-recsys/`.
