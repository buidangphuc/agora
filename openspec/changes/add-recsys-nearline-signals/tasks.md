# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/nearline/signals.py` (`NearlineSignalAggregator` and `NearlineSignalStore`).
- [x] Implement `recsys/nearline/__init__.py`.
- [x] Write unit tests in `platform-recsys/tests/test_nearline.py`.

## 2. Code — the layer had no event source and a broken Redis path (found 2026-10-09)
- [x] Aggregator Redis path: write co-views per session (it wrote none), bound recents (50) and co-views (50), ignore
      impressions for recents, replay guard by event id, ignore events older than the window. Tests with fakeredis.
- [x] `analytics.events` consumer: protobuf wire decoder (`recsys/nearline/wire.py`, pinned to bytes from the real
      protobuf runtime), the consumer loop (`consumer.py`: manual commits after Redis, skip undecodable) and the process
      entrypoint `python -m recsys.nearline` (`NEARLINE_*` / `KAFKA_*` settings, `confluent-kafka`).
- [x] Document the Redis key/value contract and the compose service (design.md); README section.

## 3. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-nearline-signals --strict`).
- [x] Run `pytest -v tests/test_nearline.py` in `platform-recsys/`.

## 4. Code — team-ai (reconciliation 2026-10-09: the reader was never wired)
- [x] `RedisNearlineStore` reading `recs:nearline:ctr:<listing_id>` (contract in design.md), built by `build_recommendation_service`
      from `RECS_NEARLINE_REDIS_URL`; the service reads it per request into a snapshot for the GBDT ranker.
- [x] Tests through `build_recommendation_service` and for the store (`test_factory_serving_wiring.py`, `test_redis_nearline_store.py`).
- [x] Compose: team-ai `RECS_NEARLINE_REDIS_URL=redis://redis:6379/0` (integrator).
- [x] Recsys consumer writes the contract's keys (platform-recsys, branch `port/mlr`); reconcile design.md with it.

- [x] `tests/test_nearline.py`, `test_nearline_wire.py`, `test_nearline_consumer.py` in `make test-host`.
- [x] e2e (`platform-e2e` `features/recommendations/mlr_nearline.feature`, step prefix `mlr_`): the real job image
      consumes beacons posted through the gateway. Needs the rebuilt `platform-recsys` image.

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
