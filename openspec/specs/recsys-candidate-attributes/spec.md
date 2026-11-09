# recsys-candidate-attributes Specification

## Purpose
TBD - created by archiving change recsys-candidate-attributes. Update Purpose after archive.

## Requirements

### Requirement: Item payloads carry seller, category and visibility

`publish_generation` SHALL read the latest `item_attributes@v2` snapshot and SHALL write `seller_id`, `category_id` and
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

#### Scenario: Missing snapshot does not abort the publish

- **WHEN** a generation is published and no `item_attributes@v2` snapshot exists
- **THEN** the publish succeeds, no payload has the three keys and the log has `attributes=missing`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_missing_snapshot_publishes_without_attributes`

#### Scenario: Unspecified or unrecognised status writes no visible

- **WHEN** a generation is published and listing `L7` has snapshot `status` null because the export said `LISTING_STATUS_UNSPECIFIED`
- **THEN** the point of `L7` has no `visible` key
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_unknown_status_writes_no_visible`

#### Scenario: Corrupt snapshot does not abort the publish

- **WHEN** a generation is published and the `item_attributes@v2` snapshot file is not readable Parquet
- **THEN** the publish succeeds, no payload has the three keys and the log has `attributes=missing`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_corrupt_snapshot_publishes_without_attributes`

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

### Requirement: A stale snapshot warns and still publishes

WHEN the `item_attributes@v2` snapshot `as_of` is older than 24 hours at publish or backfill time, THE SYSTEM SHALL log a
warning containing that `as_of` and SHALL continue. `visible` MAY lag a moderation change by at most one cycle of
export, featurestore run, and publish or backfill.

#### Scenario: Snapshot older than 24 hours warns

- **WHEN** a generation is published with a snapshot whose `as_of` is 25 hours old
- **THEN** the publish succeeds and the log has one warning containing that `as_of`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_stale_snapshot_warns_and_publishes`

#### Scenario: Snapshot of exactly 24 hours does not warn

- **WHEN** a generation is published with a snapshot whose `as_of` is exactly 24 hours old
- **THEN** the log has no staleness warning
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_snapshot_at_24_hours_does_not_warn`

### Requirement: Cache entries carry seller, category and visibility

Every entry (`{listing_id, score}`) of every Redis user, item and popular key written by `load_cache` and
`load_legacy_cache` SHALL carry `seller_id`, `category_id` and `visible` under the same rules as the point payload
(absent when unknown). The loader SHALL NOT write `in_stock`. Keys, TTLs, ordering and `score` SHALL be unchanged.

#### Scenario: Popular entries carry attributes

- **WHEN** a generation is published with popular list `[L1, L2]` where `L2` is banned
- **THEN** `recs:v1:gen:<gen>:popular` has `L1` with `visible=true` and `L2` with `visible=false`, each with its `seller_id` and `category_id`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_popular_entries_carry_attributes`

#### Scenario: User and item entries and legacy keys carry attributes

- **WHEN** a generation is published with `RECS_WRITE_LEGACY_KEYS` on
- **THEN** every entry in the generation `user` and `item` keys and in the unscoped `recs:v1:{user,item,popular}` keys carries the same attribute keys as the point payload of that listing
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_user_item_and_legacy_entries_carry_attributes`

#### Scenario: Entry with unknown attributes keeps the old shape

- **WHEN** a listing has no snapshot row
- **THEN** its cache entry is exactly `{listing_id, score}`
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_entry_without_attributes_is_the_old_shape`

#### Scenario: Entries add at most three keys and stay small

- **WHEN** a generation is published with `TOP_N` entries per key
- **THEN** each entry has at most 3 keys beyond `listing_id` and `score` and each encoded value is under 65536 bytes
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_entries_add_at_most_three_keys_and_stay_under_64kib`

#### Scenario: Qdrant writes use batches of 256

- **WHEN** 600 item points are upserted
- **THEN** the client receives 3 upsert calls of 256, 256 and 88 points
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_upsert_batches_of_256`

#### Scenario: TTL and ordering are untouched

- **WHEN** a generation is published
- **THEN** each key's TTL equals `CACHE_TTL_SECONDS` and the `listing_id` order and `score` values equal those of a publish without attributes
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_ttl_order_and_scores_unchanged`

### Requirement: Consumers treat absent visibility as not eligible

The payload contract SHALL be: `visible` absent or false means the candidate is NOT eligible; `seller_id` absent means the
candidate is never deduplicated by seller; `category_id` absent means the candidate has no category. A consumer reading
a payload written before this change SHALL therefore see no eligible candidate until the generation is rewritten or
backfilled. The consumer-side behaviour is specified and tested in `recsys-cart-cross-sell`.

#### Scenario: Contract is visible end to end

- **WHEN** a generation is published with a published listing `P` and a banned listing `B`, both similar to the cart context listing, and `Recommend` for `cart_cross_sell` is called through the gateway
- **THEN** the response contains `P` and does not contain `B`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature` `Contract is visible end to end`

#### Scenario: Payload without visible is not eligible

- **WHEN** a generation holds a point or entry for a listing with no `visible` key and `Recommend` is called
- **THEN** that listing is not in the response
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature` `Payload without visible is not eligible`

### Requirement: Existing generations are backfilled by an explicit command

The command `python -m recsys backfill-attributes` SHALL read the latest `item_attributes@v2` snapshot, then for the
serving and previous generation (from `recs:v1:serving` and `recs:v1:previous`) set the three keys on every item point
with Qdrant `set_payload` and rewrite every Redis user, item and popular entry of those generations and of the legacy unscoped `recs:v1:{user,item,popular}` keys, keeping each key's
remaining TTL (a key with no TTL stays without one; a key that expired between scan and rewrite is skipped). It SHALL NOT change vectors, scores, order, pointers or any other generation. It SHALL be idempotent and
SHALL exit 0 printing counts only (no ids). WHEN no snapshot exists the command SHALL exit 2 without writing. WHEN no
serving generation exists the command SHALL exit 0 reporting zero.

#### Scenario: Backfill patches the serving generation

- **WHEN** the serving generation was published before this change and the command runs with a snapshot
- **THEN** every item point and cache entry of the serving generation carries the attribute keys and the pointers, vectors and scores are byte-identical to before
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_patches_serving_generation`

#### Scenario: Backfill is idempotent and keeps TTL

- **WHEN** the command runs twice
- **THEN** the second run changes no value and each Redis key's TTL after both runs is at most its TTL before and more than zero
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_is_idempotent_and_keeps_ttl`

#### Scenario: Backfill without a snapshot refuses

- **WHEN** the command runs and no `item_attributes@v2` snapshot exists
- **THEN** it exits 2 and no Qdrant or Redis write occurred
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_without_snapshot_exits_2`

#### Scenario: Backfill patches legacy keys and handles TTL edge cases

- **WHEN** the command runs with legacy keys present, one of them without TTL and one that expires between scan and rewrite
- **THEN** the legacy entries carry the attribute keys, the key without TTL still has no TTL and the expired key is skipped without error
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_legacy_keys_and_ttl_edges`

#### Scenario: Backfill with a corrupt snapshot refuses

- **WHEN** the command runs and the snapshot file is not readable Parquet
- **THEN** it exits 2 and no Qdrant or Redis write occurred
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_corrupt_snapshot_exits_2`

#### Scenario: Backfill without store credentials fails closed

- **WHEN** the command runs with an invalid Redis password
- **THEN** it exits non-zero and no Qdrant write occurred
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_without_credentials_writes_nothing`

#### Scenario: Backfill leaves other generations alone

- **WHEN** a generation that is neither serving nor previous exists and the command runs
- **THEN** its points and keys are unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_touches_only_serving_and_previous`

#### Scenario: Backfill with no serving generation

- **WHEN** `recs:v1:serving` is unset and the command runs
- **THEN** it exits 0, reports zero points and zero entries and writes nothing
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_without_serving_generation`

#### Scenario: Logs carry counts and no ids

- **WHEN** the publish and the backfill run on listings with known seller ids and the logs are captured
- **THEN** no log line contains any seller id or listing id
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_logs_carry_no_ids`
