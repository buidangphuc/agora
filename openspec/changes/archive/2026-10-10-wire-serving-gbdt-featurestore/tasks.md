# Tasks

> **Reality check 2026-09-20** — tasks below were re-verified against the code. One task was
> ticked without being done; it is un-ticked and annotated. The rest hold up.

## 1. Code — team-ai
- [x] Define feature-store and ranker ports in `recommend/` (Protocol, in-memory adapter).
- [x] Inject both into `RecommendationService` via `factory.py`.
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
- [x] **The proof does not reach production.**
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

## 3. Reconciliation addendum (2026-10-09)
- [x] Serving reads the registry's `item_popularity` names (`recommend/features.py`); defaults are counted in `explain["feature_defaults"]`.
- [x] Test that fails on the old names (`test_item_feature_contract.py`); registry-parity test.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.

## Follow-ups (not done in this change)

- platform-recsys trainer aligned with the feature contract in design.md (platform-recsys, not done here).

platform-recsys has no GBDT trainer to align. The exact feature contract the trainer must follow is in design.md.
