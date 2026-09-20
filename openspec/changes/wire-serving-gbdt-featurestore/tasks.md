# Tasks

> **Reality check 2026-09-20** — tasks below were re-verified against the code. One task was
> ticked without being done; it is un-ticked and annotated. The rest hold up.

## 1. Code — team-ai
- [x] Define feature-store and ranker ports in `recommend/` (Protocol, in-memory adapter).
- [ ] Inject both into `RecommendationService` via `factory.py`.
      **Not done.** `build_recommendation_service` (`factory.py:28-58`) passes only `backend`,
      `cache` and four scalars. `RecommendationService.__init__` (`service.py:40-54`) *does*
      accept `feature_store=` / `ranker=` / `nearline_store=`, but nothing supplies them — so
      production always gets a permanently empty `InMemoryFeatureStore()` and a
      `GBDTRankerAdapter()` with hardcoded weights. `featurestore_hit_count` is therefore
      always 0 in production.
- [x] Branch on `config.ranking_model`: `"gbdt"` → `GBDTRanker.rank_candidates`; else cosine.
- [x] Branch on `config.use_featurestore`: enrich candidates, count hits.
- [x] Populate `explain` with `ranking_model`, `featurestore_hit_count`, `ranking_source`.
- [x] Add startup validation rejecting placements whose declared capability has no binding.
- [x] Ranker/feature-store failure → cosine fallback + `status: "degraded"`.

## 2. Verification — execution proof, not module test
- [x] Test calls `service.recommend(home_feed_query)` end to end and asserts
      `explain["ranking_model"] == "gbdt"` and `explain["featurestore_hit_count"] > 0`.
- [ ] **The proof does not reach production.**
      `tests/unit/modules/recommend/test_placement_engine.py:64` constructs
      `RecommendationService` directly with an injected store and asserts
      `explain["featurestore_hit_count"] == 2`. It proves the service works *when injected*; it
      cannot observe that `factory.py` never injects. A test that went through
      `build_recommendation_service` would have failed. Add one.
- [x] Test asserts returned order **differs** from pure-cosine order on a fixture where GBDT
      weights invert two candidates — the order itself is the proof the ranker ran.
- [x] Test asserts a placement declaring an unbound model fails at startup.
- [x] Test asserts ranker failure yields cosine order + `status == "degraded"`.
- [x] `openspec validate wire-serving-gbdt-featurestore --strict`.
- [x] `PYTHONPATH=. pytest tests/` in `team-ai` (**full suite**, not selected files).
