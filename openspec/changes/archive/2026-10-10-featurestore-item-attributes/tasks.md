# Tasks

## 1. Code - team-analytics
- [x] `listing_sellers` keeps `category_id` and `price` (migration, record, upsert, consumer decode); export writes `listing_sellers.parquet`; consumer group default bumped; README/.env.example. verify: `make check`, new Go tests fail without the change.

## 2. Code - platform-featurestore
- [x] `inputs.py` loads `listings` (optional file, AS_OF filter, column guard). verify: unit tests for missing file, missing column, AS_OF.
- [x] Registry: `item_attributes@v1`, `user_preferences@v1` (SQL, features.yaml, lock). verify: unit tests for values, ordering, weights, window, unknown = NULL, parity.
- [x] README contract tables. verify: `pytest`, `ruff check .`.

## 3. Code - platform-recsys
- [x] Settings (`ITEM_ATTRIBUTES_*`, `USER_PREFERENCES_*`, `TWO_TOWER_REQUIRE_ATTRIBUTES`, `TWO_TOWER_MAX_CATEGORIES`) + `.env.example`. verify: `tests/test_env_drift.py`.
- [x] `two_tower/features.py`, `stage.py`, `pipeline.py`: attributes into catalogue, features, vocabulary, lineage. verify: unit tests that fail without the change (cold item differs by category; vocabulary; required snapshot; lineage).

## 4. E2E - platform-e2e (real job images; FEATURES `planned`, note `needs rebuild`)
- [x] `featurestore/item_attributes.feature` (real analytics export + synthetic inputs) and `recommendations/recsys_item_attributes.feature`; FEATURES.yaml entries in platform-featurestore, team-analytics, platform-recsys.

## Evidence (2026-10-10)

- Final gate on HEAD 6b714354: parallel lane (`e2e.sh -q -n 4 -m "not destructive"`) 825/825 passed, run twice; destructive lane 103/104. The one failure, `test_c1_notification_faults::test_name_lookup_failure_still_notifies`, belongs to change C1 (not this change) and passed on isolated rerun (flaky). Stack READY.
