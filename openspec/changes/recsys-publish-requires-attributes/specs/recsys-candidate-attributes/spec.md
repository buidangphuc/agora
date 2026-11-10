## REMOVED Requirements

### Requirement: Item payloads carry seller, category and visibility

**Reason**: Its scenarios "Missing snapshot does not abort the publish" and "Corrupt snapshot does not abort the publish" let a generation without `visible` reach serving, where team-ai treats every candidate as not eligible and serves nothing.

**Migration**: Replaced by "Item payloads of a trained generation carry seller, category and visibility" (same payload rules, without the two scenarios) and "A run without a readable attribute snapshot refuses before training".

## ADDED Requirements

### Requirement: Item payloads of a trained generation carry seller, category and visibility

`publish_generation` SHALL read the latest `item_attributes@v2` snapshot (a run without a readable snapshot never
reaches it, see "A run without a readable attribute snapshot refuses before training") and SHALL write `seller_id`, `category_id` and
`visible` into the payload of every point of the generation's item collections (`item_als_vectors__<gen>` and
`item_two_tower_vectors__<gen>`) beside `listing_id`, `model_version`, `updated_at`. `visible` SHALL be the boolean
`status == LISTING_STATUS_PUBLISHED`, written for every listing whose status is known. An attribute that is unknown SHALL
be absent from the payload, never an empty string. Values longer than 128 characters SHALL be treated as unknown. Points
of the user collection SHALL NOT receive these keys. The loader SHALL NOT write `in_stock`.

#### Scenario: A published listing carries its attributes

- **WHEN** a generation is published and listing `L1` has seller `S1`, category `C1` and status `LISTING_STATUS_PUBLISHED`
- **THEN** the Qdrant point of `L1` in both item collections has payload `seller_id=S1`, `category_id=C1`, `visible=true`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_point_payload_carries_attributes`

#### Scenario: A banned listing is written invisible

- **WHEN** a generation is published and listing `L2` has status `LISTING_STATUS_BANNED`
- **THEN** the point of `L2` has `visible=false`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_non_published_status_is_not_visible`

#### Scenario: Unknown attribute is absent

- **WHEN** a generation is published and listing `L3` has seller `S3`, no category and no status
- **THEN** the point of `L3` has `seller_id=S3` and has no `category_id` key and no `visible` key
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_unknown_attributes_are_absent_not_empty`

#### Scenario: Listing missing from the snapshot gets no attributes

- **WHEN** a generation is published and listing `L4` has a vector but no row in the snapshot
- **THEN** the point of `L4` has none of `seller_id`, `category_id`, `visible` and the publish succeeds
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_listing_without_snapshot_row`

#### Scenario: Unspecified or unrecognised status writes no visible

- **WHEN** a generation is published and listing `L7` has snapshot `status` null because the export said `LISTING_STATUS_UNSPECIFIED`
- **THEN** the point of `L7` has no `visible` key
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_unknown_status_writes_no_visible`

#### Scenario: Oversized value is treated as unknown

- **WHEN** listing `L5` has a `seller_id` of 129 characters
- **THEN** the point of `L5` has no `seller_id` key and the other known attributes are written
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_value_over_128_characters_is_unknown`

#### Scenario: Exactly 128 characters is kept

- **WHEN** listing `L6` has a `seller_id` of exactly 128 characters
- **THEN** the point of `L6` has that `seller_id`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_value_of_128_characters_is_kept`

#### Scenario: User points are not enriched

- **WHEN** a generation is published
- **THEN** no point of `user_als_vectors__<gen>` has a `seller_id`, `category_id` or `visible` key
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_user_points_unchanged`

### Requirement: A run without a readable attribute snapshot refuses before training

A `python -m recsys` training run SHALL resolve the `item_attributes@v2` snapshot (`ITEM_ATTRIBUTES_V2_PATH`, else the
latest `as_of=<YYYYMMDDTHHMMSSZ>.parquet` under `ITEM_ATTRIBUTES_V2_DIR`) before it reads the dataset or trains. WHEN no
snapshot exists or the snapshot is not readable Parquet with a `listing_id` column, the run SHALL exit 2 and log
`attributes=missing`, SHALL NOT start an MLflow run, SHALL NOT write any Qdrant collection or Redis key and SHALL NOT move
`recs:v1:serving`. A stale snapshot (older than 24 hours) SHALL NOT trigger this refusal. The `rollback` and
`backfill-attributes` commands are not training runs and keep their own rules.

#### Scenario: Missing snapshot refuses the run

- **WHEN** a training run starts and no `item_attributes@v2` snapshot exists
- **THEN** it exits 2, the log has `attributes=missing`, no MLflow run was created, no Qdrant collection or Redis key was written and `recs:v1:serving` is unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_missing_snapshot_refuses_before_training`

#### Scenario: Corrupt snapshot refuses the run

- **WHEN** a training run starts and the `item_attributes@v2` snapshot file is not readable Parquet
- **THEN** it exits 2, the log has `attributes=missing` and nothing is trained or written
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_corrupt_snapshot_refuses_before_training`

#### Scenario: A refused run keeps serving the previous generation

- **WHEN** generation `G1` is serving and a training run starts without a readable snapshot
- **THEN** the run exits 2 and Recommend through the gateway still answers from `G1` with a non-empty list
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature` `A refused run keeps serving the previous generation`

#### Scenario: A stale snapshot still trains

- **WHEN** a training run starts and the only snapshot is 30 hours old
- **THEN** the run trains and publishes, and the log has one stale-snapshot warning naming its `as_of`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_stale_snapshot_still_trains`
