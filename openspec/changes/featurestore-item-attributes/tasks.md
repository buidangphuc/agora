# Tasks

## 1. Code - team-analytics
- [ ] `listing_sellers` keeps `category_id` and `price` (migration, record, upsert, consumer decode); export writes `listing_sellers.parquet`; consumer group default bumped; README/.env.example. verify: `make check`, new Go tests fail without the change.

## 2. Code - platform-featurestore
- [ ] `inputs.py` loads `listings` (optional file, AS_OF filter, column guard). verify: unit tests for missing file, missing column, AS_OF.
- [ ] Registry: `item_attributes@v1`, `user_preferences@v1` (SQL, features.yaml, lock). verify: unit tests for values, ordering, weights, window, unknown = NULL, parity.
- [ ] README contract tables. verify: `pytest`, `ruff check .`.

## 3. Code - platform-recsys
- [ ] Settings (`ITEM_ATTRIBUTES_*`, `USER_PREFERENCES_*`, `TWO_TOWER_REQUIRE_ATTRIBUTES`, `TWO_TOWER_MAX_CATEGORIES`) + `.env.example`. verify: `tests/test_env_drift.py`.
- [ ] `two_tower/features.py`, `stage.py`, `pipeline.py`: attributes into catalogue, features, vocabulary, lineage. verify: unit tests that fail without the change (cold item differs by category; vocabulary; required snapshot; lineage).

## 4. E2E - platform-e2e (real job images; FEATURES `planned`, note `needs rebuild`)
- [ ] `featurestore/item_attributes.feature` (real analytics export + synthetic inputs) and `recommendations/recsys_item_attributes.feature`; FEATURES.yaml entries in platform-featurestore, team-analytics, platform-recsys.
