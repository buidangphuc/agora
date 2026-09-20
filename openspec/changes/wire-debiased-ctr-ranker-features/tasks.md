# Tasks

## 1. Code — team-ai
- [x] Integrate `NearlineSignalPort` into `recommend/ranking.py`.
- [x] Sourced `historical_ctr` from nearline position-debiased CTR in `GBDTRankerAdapter`.
- [x] Inject `nearline_store` into `RecommendationService` and wire into serving enrichment.
- [x] Record `ctr_source` provenance ("nearline" vs "fallback").
- [x] Write execution-proof tests in `tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.

## 2. Verification
- [x] Run `pytest -v tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.
- [x] `openspec validate wire-debiased-ctr-ranker-features --strict`.
