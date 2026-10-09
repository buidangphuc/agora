# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/nearline/signals.py` (`NearlineSignalAggregator` and `NearlineSignalStore`).
- [x] Implement `recsys/nearline/__init__.py`.
- [x] Write unit tests in `platform-recsys/tests/test_nearline.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-nearline-signals --strict`).
- [x] Run `pytest -v tests/test_nearline.py` in `platform-recsys/`.

## 3. Code — team-ai (reconciliation 2026-10-09: the reader was never wired)
- [ ] `RedisNearlineStore` reading `recs:nearline:ctr:<listing_id>` (contract in design.md), built by `build_recommendation_service`
      from `RECS_NEARLINE_REDIS_URL`; the service reads it per request into a snapshot for the GBDT ranker.
- [ ] Tests through `build_recommendation_service` and for the store (`test_factory_serving_wiring.py`, `test_redis_nearline_store.py`).
- [ ] Compose: team-ai `RECS_NEARLINE_REDIS_URL=redis://redis:6379/0` (integrator).
- [ ] Recsys consumer writes the contract's keys (platform-recsys, branch `port/mlr`); reconcile design.md with it.
