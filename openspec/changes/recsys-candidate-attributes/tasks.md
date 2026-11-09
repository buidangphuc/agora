Depends on slice analytics-listing-status-export (status column export, one-off replay C15). Deploy order: analytics replay, featurestore v2, publish/backfill, consumer. Merge order: unit tracks, then the consumer, then the e2e track.

## 1. Code track: platform-featurestore

- [x] 1.1 Tests first, verify red: `tests/test_item_attributes.py` (`test_v2_carries_status`, `test_v2_without_status_column_is_null_not_an_error`, `test_v1_definition_is_unchanged`, `test_v2_listing_after_as_of_excluded`, `test_v2_unspecified_or_unknown_status_is_null`, `test_current_is_2_and_both_versions_exist`), `tests/test_materialize.py` (`test_v2_drift_is_refused`); update `tests/test_item_attributes.py:65` (`fs:item_attributes:current` becomes `2`)
- [x] 1.2 `featurestore/inputs.py`: read the optional `status` column into the `listings` temp table (NULL when absent, warning naming `status`); do not add it to `_require_listing_columns` (Export without a status column leaves status null)
- [x] 1.3 `registry/sql/item_attributes_v2.sql` + entry `item_attributes` version 2 in `registry/features.yaml`; regenerate `registry/features.lock` with `python -m featurestore lock`; v1 SQL and hash untouched (Status reaches the v2 snapshot and online key, v1 is unchanged, A changed definition without a version bump is rejected, A listing changed after AS_OF is excluded)
- [x] 1.4 README: document v2 and the optional `status` column

## 2. Code track: platform-recsys

- [x] 2.1 Tests first, verify red: `tests/test_candidate_attributes.py` with all scenario names (including stale snapshot, corrupt snapshot, unknown status, legacy backfill, credentials, 3-key, 256-batch and 64 KiB checks) of `specs/recsys-candidate-attributes/spec.md`
- [x] 2.2 `recsys/config.py`: `ITEM_ATTRIBUTES_V2_DIR` (default `/features/item_attributes/v2`) and `ITEM_ATTRIBUTES_V2_PATH` (default empty); update `.env.example`; add the path to the compose volume mounts if the v1 dir is mounted; no enable flag
- [x] 2.3 `recsys/load/attributes.py`: build `{listing_id: {seller_id?, category_id?, visible?}}` from the snapshot with the 128-character rule (A published listing carries its attributes, A banned listing is written invisible, Unknown attribute is absent, Listing missing from the snapshot gets no attributes, Missing snapshot does not abort the publish, Oversized value is treated as unknown, Exactly 128 characters is kept)
- [x] 2.4 `recsys/load/qdrant.py` `_upsert` and `redis_cache.py` `_encode`/`load_cache`/`load_legacy_cache` take the attribute map; user points untouched; `publish.py` and `pipeline.py` resolve the snapshot (optional) and pass it (User points are not enriched, Popular entries carry attributes, User and item entries and legacy keys carry attributes, Entry with unknown attributes keeps the old shape, TTL and ordering are untouched)
- [x] 2.5 `recsys/backfill.py` and the `backfill-attributes` branch in `recsys/__main__.py` (update the unknown-command message): Qdrant `set_payload` in batches of 256 on the serving and previous item collections, Redis rewrite preserving TTL (`PTTL` then `SET ... PX`) (Backfill patches the serving generation, Backfill is idempotent and keeps TTL, Backfill without a snapshot refuses, Backfill leaves other generations alone, Backfill with no serving generation, Logs carry counts and no ids)
- [x] 2.6 Update the module docstrings of `load/qdrant.py` and `load/redis_cache.py` (entries are no longer "ids + scores only") and the README

## 3. E2E track

- [x] 3.1 `platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature`: `Contract is visible end to end`, `Payload without visible is not eligible` (needs `recsys-cart-cross-sell` merged for the gateway-entry assertions; seed by running publish on a fixture generation)
- [x] 3.2 FEATURES.yaml entries via the e2e-author (platform-recsys, platform-featurestore)
- [ ] 3.3 Local run with only redis, qdrant, team-ai, gateway; measure publish time delta, non-gating
- [x] 3.4 `openspec validate recsys-candidate-attributes --strict`, per-repo checks, pre-commit-ci
