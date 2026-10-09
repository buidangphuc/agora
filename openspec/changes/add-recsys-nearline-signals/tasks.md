# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/nearline/signals.py` (`NearlineSignalAggregator` and `NearlineSignalStore`).
- [x] Implement `recsys/nearline/__init__.py`.
- [x] Write unit tests in `platform-recsys/tests/test_nearline.py`.

## 2. Code — the layer had no event source and a broken Redis path (found 2026-10-09)
- [ ] Aggregator Redis path: write co-views per session (it wrote none), bound recents (50) and co-views (50), ignore
      impressions for recents, replay guard by event id, ignore events older than the window. Tests with fakeredis.
- [ ] `analytics.events` consumer: protobuf wire decoder (`recsys/nearline/wire.py`, pinned to bytes from the real
      protobuf runtime), the consumer loop (`consumer.py`: manual commits after Redis, skip undecodable) and the process
      entrypoint `python -m recsys.nearline` (`NEARLINE_*` / `KAFKA_*` settings, `confluent-kafka`).
- [ ] Document the Redis key/value contract and the compose service (design.md); README section.

## 3. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-nearline-signals --strict`).
- [x] Run `pytest -v tests/test_nearline.py` in `platform-recsys/`.
- [ ] `tests/test_nearline.py`, `test_nearline_wire.py`, `test_nearline_consumer.py` in `make test-host`.
- [ ] e2e (`platform-e2e` `features/recommendations/mlr_nearline.feature`, step prefix `mlr_`): the real job image
      consumes beacons posted through the gateway. Needs the rebuilt `platform-recsys` image.
