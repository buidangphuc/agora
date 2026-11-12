## MODIFIED Requirements

### Requirement: Item payloads of a trained generation carry seller, category and visibility

`publish_generation` SHALL read the latest `item_attributes@v2` snapshot (a run without a readable snapshot never
reaches it, see "A run without a readable attribute snapshot refuses before training") and SHALL write `seller_id`, `category_id` and
`visible` into the payload of every point of the generation's item collections (`item_als_vectors__<gen>` and
`item_two_tower_vectors__<gen>`) beside `listing_id`, `model_version`, `updated_at`. `visible` SHALL be the boolean
`status == LISTING_STATUS_PUBLISHED`, written for every listing whose status is known; `LISTING_STATUS_DELETED` is a known status, so a deleted listing has `visible=false` (present, not absent). An attribute that is unknown SHALL
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

#### Scenario: A deleted listing is written invisible

- **WHEN** a generation is published and listing `L8` has snapshot `status = LISTING_STATUS_DELETED`
- **THEN** the point of `L8` in both item collections has `visible=false` and the key is present
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_deleted_status_is_not_visible`

#### Scenario: A deleted listing is not recommended end to end

- **WHEN** a generation is published with a published listing `P` and a deleted listing `X`, both similar to the cart context listing, and `Recommend` for `cart_cross_sell` is called through the gateway
- **THEN** the response contains `P` and does not contain `X`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/candidate_attributes.feature` "A deleted listing is not recommended"

#### Scenario: Backfill writes visible=false for a DELETED snapshot row

- **WHEN** `backfill-attributes` runs on the serving generation with a snapshot row `L8` of `status = LISTING_STATUS_DELETED`
- **THEN** the item points of `L8` and every Redis user, item and popular entry for `L8` of that generation carry `visible=false` (key present)
- **VERIFIED BY**: `platform-recsys/tests/test_candidate_attributes.py` `test_backfill_deleted_status_writes_visible_false`
