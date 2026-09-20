# Tasks

## 1. Code — team-ai
- [x] Define feature-store and ranker ports in `recommend/` (Protocol, in-memory adapter).
- [x] Inject both into `RecommendationService` via `factory.py`.
- [x] Branch on `config.ranking_model`: `"gbdt"` → `GBDTRanker.rank_candidates`; else cosine.
- [x] Branch on `config.use_featurestore`: enrich candidates, count hits.
- [x] Populate `explain` with `ranking_model`, `featurestore_hit_count`, `ranking_source`.
- [x] Add startup validation rejecting placements whose declared capability has no binding.
- [x] Ranker/feature-store failure → cosine fallback + `status: "degraded"`.

## 2. Verification — execution proof, not module test
- [x] Test calls `service.recommend(home_feed_query)` end to end and asserts
      `explain["ranking_model"] == "gbdt"` and `explain["featurestore_hit_count"] > 0`.
- [x] Test asserts returned order **differs** from pure-cosine order on a fixture where GBDT
      weights invert two candidates — the order itself is the proof the ranker ran.
- [x] Test asserts a placement declaring an unbound model fails at startup.
- [x] Test asserts ranker failure yields cosine order + `status == "degraded"`.
- [x] `openspec validate wire-serving-gbdt-featurestore --strict`.
- [x] `PYTHONPATH=. pytest tests/` in `team-ai` (**full suite**, not selected files).
