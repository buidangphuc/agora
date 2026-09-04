# Tasks

Depends on **add-recommendation-contract** (generated `recommendation_pb2*` stubs vendored into
team-ai) and **build-als-training-job** (Qdrant collection + Redis pre-computed lists populated).
Both must land before this change can run end-to-end; the code below can be written against the
assumed contract shapes in `design.md` and compiled once the stubs exist.

## 1. Code — team-ai (config + module)
- [x] `app/core/config/recommendations.py` (new): `RecommendationSettingsMixin` with
      `RECS_ENABLED` (default `false`), `RECS_BACKEND` (`"qdrant" | "memory"`, default `memory`),
      `RECS_QDRANT_COLLECTION` (default `item_als_vectors`), `RECS_VECTOR_DIM`,
      `RECS_CANDIDATE_TOP_K` (default 100), `RECS_RESULT_TOP_K` (default 10),
      `RECS_CACHE_PREFIX` (default `recs`), `RECS_CACHE_SCHEMA_VERSION`,
      `RECS_CACHE_TTL_SECONDS`, `RECS_RETRIEVE_TIMEOUT_MS` — mirror the `RAG_*` block in
      `app/core/config/ai.py`. Mix into `Settings`.
      (Also added `RECS_QDRANT_URL`, `RECS_QDRANT_DISTANCE`, `RECS_MODEL_VERSION`; mixed into
      `Settings` in `app/core/config/__init__.py`. Verified by `test_recommend_wiring.py`.)
- [x] `app/modules/business/recommend/service.py` (new): `RecommendationService` implementing the
      two-stage pipeline — Redis pre-computed fast path, Qdrant ANN candidate retrieval (Top-100),
      ranking + business-rule filtering (in-stock, dedupe, drop seed) → Top-10, cold-start /
      anonymous / popularity fallback. Reuse the Qdrant client from `app/modules/ai/rag` and the
      Redis client from `app/core/redis.py`.
      (Stage-2 filtering factored into `ranking.py`; cache reader in `cache.py`; proto-free
      schemas in `schemas.py`. Redis client from `app/core/redis.py`; Qdrant client via a lazy
      import in `backends.py`.)
- [x] `app/modules/business/recommend/backends.py` (new): `qdrant` and `memory` retrieval backends
      behind one interface (mirror the `RAG_BACKEND` seam); `memory` returns deterministic
      fixtures for offline dev / e2e.
- [x] Startup collection check: on module open, verify the Qdrant collection name/dim/metric
      matches config; on mismatch log and make `Recommend` return `UNAVAILABLE` (contract break
      with the training job is loud, not a silent empty result).
      (`backend.collection_ok()` run in `factory.build_recommendation_service`; on mismatch the
      service carries `collection_ok=False` and `recommend()` raises `ServiceUnavailableError`,
      which the servicer maps to `UNAVAILABLE`. Verified by `test_collection_mismatch_raises_unavailable`.)

## 2. Code — team-ai (transport + wiring)
- [x] `app/transport/grpc/servicers/recommend.py` (new): `RecommendationServicer` implementing
      `recommendation_pb2_grpc.RecommendationServiceServicer` — thin transport (parse,
      `ensure_scopes`, call module, map to `RecommendResponse`); abort `UNAVAILABLE` when the
      module provider returns `None` (RECS_ENABLED=false), mirroring `search.py`.
      (Written against the assumed contract shape; NOT import-run locally — imports generated
      `recommendation_pb2` which needs buf/CI. Self-reviewed against `search.py`: same
      `ensure_scopes` → provider-None `UNAVAILABLE` → `ServiceUnavailableError` → `UNAVAILABLE`
      shape. Scope `recommendations:read`.)
- [x] `app/transport/grpc/server.py`: import the servicer + `recommendation_pb2_grpc`, add a
      `recommendation_provider` param to `build_grpc_server`, and register
      `add_RecommendationServiceServicer_to_server(...)` next to Search/AI/Chat.
      (Registration is defensive — wrapped in `_register_recommendation_service`, which skips with
      a log line when the generated stubs are absent so existing server/tests stay green until the
      contract lands; in CI with the stubs present it always registers and gates via the provider.)
- [x] `app/bootstrap/application.py` / resources: build the `RecommendationService` (wired to
      Qdrant + Redis) when `RECS_ENABLED=true`, provide it to `build_grpc_server`; provide `None`
      when disabled.
      (Built by `RecommendAddon` (registered in `app/bootstrap/addons.py`) which is gated on
      `RECS_ENABLED`; `recommendation_service` added to `ApplicationResources`;
      `_start_grpc_server` passes a `recommendation_provider` reading it. Provider returns `None`
      when disabled. Verified by `test_addon_is_gated_by_flag` / `test_addon_open_builds_service`.)
- [x] `docker-compose.services.yaml` (team-ai service env): document/default `RECS_ENABLED`,
      `RECS_BACKEND`, `RECS_QDRANT_COLLECTION`, cache knobs — no new port (reuses team-ai
      `GRPC_PORT`).
- [ ] Do NOT hand-edit generated `recommendation_pb2*` (Rule 4). Verify the module compiles
      against the generated stubs from add-recommendation-contract.
      (Deferred to CI — no proto hand-edited; the servicer + server registration compile against
      the generated stubs only once add-recommendation-contract vendors them. Pure module compiles
      and is tested offline today.)

## 3. Tests — team-ai
- [x] Unit: gating (`RECS_ENABLED=false` → `UNAVAILABLE`), cache hit skips Qdrant, cache miss
      falls back, in-stock/dedupe/seed filtering, anonymous+seed similar-items, no-user-no-seed
      popularity fallback, retrieval-timeout → fallback.
      (26 tests in `tests/unit/modules/recommend/`, all passing. Gating is covered at the
      addon/provider seam — `RECS_ENABLED=false` ⇒ provider `None` ⇒ servicer `UNAVAILABLE`; the
      servicer-level abort itself asserts in CI where the proto stub exists, self-reviewed vs
      `search.py`'s identical path. Collection-mismatch `UNAVAILABLE` covered directly.)
- [x] Use the `memory` backend for deterministic offline runs (no real Qdrant/Redis in unit CI).

## E2E (platform-e2e)
- [x] `team-ai/FEATURES.yaml`: add the `recommendations.serve` capability entry
      (`api.exercised: [recommendation.v1.Recommend]`, `services: [team-ai]`), `status: planned`
      — team-ai has no browser surface of its own. The user-facing `.feature` + page objects are
      owned by **surface-recommendations**, which flips this capability's coverage to `automated`.
- [x] No new `.feature` file in this change; the serving contract is asserted by the spec
      scenarios and team-ai unit tests above.

## Archive
- [ ] `RECS_ENABLED=true` boot verified: `Recommend` returns a Top-10 against the training-job
      Qdrant collection + Redis lists (both from build-als-training-job), gating returns
      `UNAVAILABLE` when off.
      (Deferred — needs the generated proto stubs + a live Qdrant collection and Redis lists from
      build-als-training-job. Offline equivalent verified with the `memory` backend + fake Redis.)
- [ ] Blocked on add-recommendation-contract + build-als-training-job landing; archive only after
      surface-recommendations' e2e is green (`make -C platform-e2e features-check`) since that is
      where the recommendations capability reaches `automated`.
