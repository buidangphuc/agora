## ADDED Requirements

### Requirement: item_attributes@v2 carries the listing status

The registry SHALL declare `item_attributes@v2`, per listing: `seller_id`, `category_id`, `price` (as v1) and `status`
(string, the `ListingStatus` enum name, for example `LISTING_STATUS_PUBLISHED`), computed as of `AS_OF` from the
point-in-time table `listings`, written to the offline snapshot and to versioned online keys with the manifest, freshness
and parity rules of the other views. `item_attributes@v1` SHALL stay byte-identical in definition and lock hash.
`status` SHALL be read from an optional `status` column of `listing_sellers.parquet`. WHEN the file has no `status`
column, THE SYSTEM SHALL write `status` as null for every listing, SHALL NOT fail the job, and SHALL log a warning naming
the column. An unknown status is null, never a substituted constant.

#### Scenario: Status reaches the v2 snapshot and online key

- **WHEN** the materialisation job runs on a `listing_sellers.parquet` whose listing `L1` has `status=LISTING_STATUS_PUBLISHED`
- **THEN** the `item_attributes@v2` row for `L1` has that `status` and the online key `fs:item_attributes:v2:L1` carries it
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v2_carries_status`

#### Scenario: Export without a status column leaves status null

- **WHEN** the job runs on a `listing_sellers.parquet` without a `status` column
- **THEN** the job exits 0, every `item_attributes@v2` row has `status` null and the log has a warning naming `status`
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v2_without_status_column_is_null_not_an_error`

#### Scenario: v1 is unchanged

- **WHEN** the job runs and the registry drift check compares `item_attributes@v1` to `registry/features.lock`
- **THEN** the hash of `item_attributes@v1` equals the locked value and the drift check exits 0
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v1_definition_is_unchanged`

#### Scenario: A changed definition without a version bump is rejected

- **WHEN** the SQL of `item_attributes@v2` is edited and `registry/features.lock` is not regenerated
- **THEN** the job exits 4 (registry drift) naming `item_attributes@v2`
- **VERIFIED BY**: `platform-featurestore/tests/test_materialize.py` `test_v2_drift_is_refused`

#### Scenario: A listing changed after AS_OF is excluded

- **WHEN** a listing's latest change is after `AS_OF`
- **THEN** it has no `item_attributes@v2` row
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v2_listing_after_as_of_excluded`

#### Scenario: Unspecified or unrecognised status is unknown

- **WHEN** the export has a listing with `status=LISTING_STATUS_UNSPECIFIED` and another with `status=LISTING_STATUS_FOO`
- **THEN** both `item_attributes@v2` rows have `status` null
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v2_unspecified_or_unknown_status_is_null`

#### Scenario: Registry flips to v2 and v1 stays readable

- **WHEN** the job runs after this change
- **THEN** `fs:item_attributes:current` is `2` and both `fs:item_attributes:v1:<id>` and `fs:item_attributes:v2:<id>` exist for a listing
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_current_is_2_and_both_versions_exist`
