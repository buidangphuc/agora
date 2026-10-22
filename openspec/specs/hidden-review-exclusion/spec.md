# hidden-review-exclusion Specification

## Purpose
Featurestore popularity and ALS datasets exclude reviews that an admin hid as of the build's AS_OF time.

## Requirements

### Requirement: Item popularity ignores hidden reviews

`item_popularity@v2` SHALL compute `review_count` and `avg_rating` from `review_created` facts that have no `review_hidden` fact with the same `review_id` visible as of `$as_of`. All other columns SHALL equal `item_popularity@v1` for the same inputs.

#### Scenario: Hidden review exclusion: a hidden review leaves review_count and avg_rating
- **WHEN** listing L1 has `review_created` facts R1 (rating 5) and R2 (rating 1), and a `review_hidden` fact for R2 occurred before AS_OF, and `item_popularity@v2` is computed
- **THEN** the L1 row has `review_count` 1 and `avg_rating` 5.0
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v2_excludes_hidden_review`

#### Scenario: Hidden review exclusion: a listing whose only review is hidden has no rating
- **WHEN** listing L1 has one `review_created` fact R1 and a `review_hidden` fact for R1 before AS_OF, plus 2 view events, and `item_popularity@v2` is computed
- **THEN** the L1 row has `review_count` 0, `avg_rating` NULL and `views_7d` 2
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v2_only_review_hidden`

#### Scenario: Hidden review exclusion: a hide recorded after AS_OF is not applied
- **WHEN** a `review_hidden` fact for R2 has `ingested_at` after AS_OF and `item_popularity@v2` is computed as of AS_OF
- **THEN** the L1 row still has `review_count` 2 and `avg_rating` 3.0 (ratings 5 and 1)
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v2_hide_after_as_of_not_applied`

#### Scenario: Hidden review exclusion: a review_hidden for an unknown review id changes nothing
- **WHEN** a `review_hidden` fact carries a `review_id` that matches no `review_created` fact and `item_popularity@v2` is computed
- **THEN** `item_popularity@v2` returns the same `review_count` and `avg_rating` for every listing as without that fact, and the job exits 0
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v2_unknown_review_id_ignored`

#### Scenario: Hidden review exclusion: a review without a review id is still counted
- **WHEN** a legacy `review_created` fact has NULL `review_id`, its listing has a `review_hidden` fact for another review id, and `item_popularity@v2` is computed (also on an export with no `review_id` column)
- **THEN** that legacy review is still counted in `review_count` (never excluded, accepted limitation)
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_old_export_without_review_id_still_loads`

#### Scenario: Hidden review exclusion: non-review columns match v1
- **WHEN** `item_popularity@v1` and `item_popularity@v2` are computed on the same inputs containing views, clicks, add-to-carts, impressions and favourites
- **THEN** `views_7d`, `clicks_7d`, `add_to_cart_7d`, `favorites_current` and `ctr_7d` are equal for every listing
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v2_non_review_columns_equal_v1`

### Requirement: ALS interactions ignore hidden reviews

`als_interactions@v3` SHALL apply the `als_interactions@v2` consent rules and SHALL NOT produce the review weight (+2.0 for rating >= 4, -2.0 for rating <= 2) for a `review_created` fact that has a `review_hidden` fact with the same `review_id` visible as of `$as_of`. The exclusion SHALL be applied before the per-(user, listing) latest-review selection.

#### Scenario: Hidden review exclusion: a hidden positive review adds no ALS weight
- **WHEN** consented user u1 has a `review_created` fact R1 (rating 5) on L1 and no other signal on L1, R1 has a `review_hidden` fact before AS_OF, and `als_interactions@v3` is built
- **THEN** the dataset has no (u1, L1) row
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_als_v3_hidden_review_adds_no_weight`

#### Scenario: Hidden review exclusion: a hidden negative review no longer lowers a weight
- **WHEN** consented user u1 has 2 `view` events (weight 2.0) and a hidden `review_created` R1 (rating 1) on L1, and `als_interactions@v3` is built
- **THEN** the (u1, L1) row has `weight` 2.0 and `interactions` 2 (the same row `als_interactions@v2` gives without R1)
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_als_v3_hidden_negative_review_not_subtracted`

#### Scenario: Hidden review exclusion: a visible earlier review of the same listing still counts
- **WHEN** consented user u1 has visible review R1 (rating 5, earlier) and hidden review R2 (rating 1, later) on L1, and `als_interactions@v3` is built
- **THEN** the (u1, L1) row has `weight` 2.0 and `interactions` 1
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_als_v3_visible_sibling_review_counts`

#### Scenario: Hidden review exclusion: a hide recorded after AS_OF is not applied to a dataset
- **WHEN** the `review_hidden` fact for R1 has `ingested_at` after AS_OF and `als_interactions@v3` is built as of AS_OF
- **THEN** the (u1, L1) row still carries the review weight 2.0, and the manifest `definition_sha256` equals the v3 definition hash
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_als_v3_hide_after_as_of_not_applied`

#### Scenario: Hidden review exclusion: v3 keeps the consent filter
- **WHEN** user u2 has no personalization consent and a visible review on L1, and `als_interactions@v3` is built
- **THEN** `als_interactions@v3` has no row for u2
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_als_v3_keeps_consent_filter`

### Requirement: Locked definitions are not edited

The registry SHALL keep `item_popularity@v1`, `als_interactions@v1` and `als_interactions@v2` byte-identical (lock hashes unchanged) and SHALL register the new definitions as `item_popularity@v2` and `als_interactions@v3`, so that a drift check passes with the regenerated lock and fails if a locked file is edited.

#### Scenario: Hidden review exclusion: new versions are registered and locked
- **WHEN** the registry is loaded and `check_lock` runs
- **THEN** `item_popularity@v2` and `als_interactions@v3` are present with lock entries, and the lock hashes of `item_popularity@v1`, `als_interactions@v1` and `als_interactions@v2` equal their previous values (82c25539..., 9fb41c13..., b8edcca4...)
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_new_versions_locked_old_hashes_unchanged`

#### Scenario: Hidden review exclusion: editing a locked file fails with drift
- **WHEN** `registry/sql/item_popularity.sql` is modified without a version bump and the job starts
- **THEN** the job exits 4 with "definition changed without a version bump"
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_editing_locked_view_exits_4`

#### Scenario: Hidden review exclusion: old versions still count hidden reviews
- **WHEN** a hidden review R2 exists and `item_popularity@v1` is computed
- **THEN** its `review_count` still includes R2 (v1 behaviour unchanged for consumers not yet moved)
- **VERIFIED BY**: platform-featurestore `tests/test_hidden_review_exclusion.py::test_popularity_v1_unchanged`
