# item-attributes Specification

## Purpose
Defines the listing attribute export from the warehouse and the item_attributes and user_preferences feature views materialised from it.

## Requirements

### Requirement: Attribute views are materialised from the listing export

The registry SHALL declare `item_attributes@v1` and `user_preferences@v1`, computed as of `AS_OF` and written to the
offline snapshot and to versioned online keys with the same manifest, freshness and parity rules as the other views.
- `item_attributes@v1`, per listing: `seller_id`, `category_id`, `price`. A listing whose latest recorded change is after
  `AS_OF` SHALL NOT appear. An attribute that is unknown is null, never a substituted constant.
- `user_preferences@v1`, per `user_key`: `preferred_categories`, the up to three categories with the highest weighted
  count of the user's views (1), clicks (2) and add-to-carts (5) on listings with a known category in the 30 days before
  `AS_OF`, ordered by weight descending then name, joined by a comma. A user with no such interaction has no row.

When `listing_sellers.parquet` does not exist the job SHALL warn and both views SHALL be empty (the other views are
unaffected). When it exists without `category_id` or `price` the job SHALL exit 2 naming the column.

#### Scenario: A listing's attributes become item features

- **WHEN** a seller creates a listing with a category and a price, the export cycle completes, and the materialisation
  job runs
- **THEN** the online features of that listing under item_attributes@v1 carry that category and price and the seller

#### Scenario: A buyer's preferred categories follow their interactions

- **WHEN** a buyer views a listing of category A three times and a listing of category B once, the export cycle completes,
  and the materialisation job runs
- **THEN** the online preferred_categories of that buyer under user_preferences@v1 is "A,B"

#### Scenario: A listing changed after AS_OF is not in the snapshot

- **WHEN** the listing export holds one listing last changed before AS_OF and one changed after it, and the job runs
- **THEN** the item_attributes@v1 snapshot has a row for the first and none for the second

#### Scenario: A tampered attribute fails the parity check

- **WHEN** after a run, one listing's item_attributes@v1 online category is overwritten with a different value, and
  python -m featurestore parity runs
- **THEN** the command exits non-zero and its output names that listing and category_id

#### Scenario: Without a listing export the attribute views are empty

- **WHEN** the job runs over inputs without listing_sellers.parquet
- **THEN** it exits 0 with a warning naming the file, and the item_attributes@v1 and user_preferences@v1 snapshots have no rows

#### Scenario: A listing export without the new columns stops the run

- **WHEN** the job runs over a listing_sellers.parquet that has no category_id column
- **THEN** it exits 2 and its output names category_id

### Requirement: A one-off replay command backfills listing status

team-analytics SHALL provide the command `listing-replay` (`cmd/listing-replay`) that reads `KAFKA_LISTING_TOPIC` from the
earliest offset to the end offsets captured at start, on its own consumer group `KAFKA_LISTING_REPLAY_GROUP` that is
different from `KAFKA_LISTING_CONSUMER_GROUP`, upserts every decodable `ListingChanged` through the normal decoder and
upsert, and exits 0 when done. A listing whose newest event is `CHANGE_TYPE_DELETED` SHALL end with status `LISTING_STATUS_DELETED`, including a listing deleted before this change. It SHALL NOT be started by the service and SHALL NOT be an enable flag. It SHALL NOT move
the offsets of the live group. It SHALL be safe to run more than once.

#### Scenario: After replay every listing with a status event has its status in the export

- **WHEN** a warehouse holds rows for listings `L1..L3` with null `status`, `listing.events` holds a status-bearing `ListingChanged` for each, `listing-replay` runs to completion and the next export cycle completes
- **THEN** `listing_sellers.parquet` has exactly one row per listing and each `status` equals the enum name of that listing's newest event
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_FillsStatusForAllListings`, `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "After replay every listing with a status event has its status in the export"

#### Scenario: Replay does not touch the live consumer group

- **WHEN** `listing-replay` runs
- **THEN** the committed offsets of `KAFKA_LISTING_CONSUMER_GROUP` are identical before and after, and the replay group differs from it
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_UsesOwnGroup`

#### Scenario: Replay twice is idempotent

- **WHEN** `listing-replay` runs a second time over the same topic
- **THEN** the table has the same rows with the same values and the command exits 0
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_Idempotent`

#### Scenario: Replay does not regress a newer live status

- **WHEN** a row has `LISTING_STATUS_BANNED` at T2 and replay consumes an older event `LISTING_STATUS_PUBLISHED` at T1 < T2
- **THEN** the stored `status` remains `LISTING_STATUS_BANNED`
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_DoesNotRegressNewerRow`

#### Scenario: Replay fails when the broker is unreachable

- **WHEN** `listing-replay` cannot reach the broker or the warehouse
- **THEN** it exits non-zero with an error and leaves the table unchanged
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_BrokerDownExitsNonZero`

#### Scenario: Replay skips a malformed record and continues

- **WHEN** the topic holds an undecodable record between two valid events
- **THEN** both valid events are applied, the bad record is logged without ids and counted, and the command exits 0
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_SkipsMalformedRecord`

#### Scenario: Replay re-derives DELETED for a listing deleted earlier

- **WHEN** a warehouse holds a row for `L1` with null `status` (seeded by a test fixture that upserts through the warehouse writer, or in e2e by running the pre-change team-analytics decoder, see task 4.1; written as the previous decoder wrote it from a `CHANGE_TYPE_DELETED` event at T2), `listing.events` holds `L1`'s `CREATED` event at T1 and `DELETED` event at T2, `listing-replay` runs to completion and the next export cycle completes
- **THEN** `listing_sellers.parquet` has exactly one row for `L1` with `status = LISTING_STATUS_DELETED` and `updated_at = T2`
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_RederivesDeletedStatus`, `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "Replay re-derives DELETED for a listing deleted earlier"

#### Scenario: Replay of a listing whose delete event is no longer on the topic leaves it unchanged

- **WHEN** a row for `L2` has null `status` and `listing.events` holds no event for `L2` (retention expired) and replay runs
- **THEN** the row of `L2` is unchanged (status still null) and the command exits 0
- **VERIFIED BY**: `team-analytics/internal/app/listing_replay_test.go` `TestListingReplay_ExpiredDeleteEventLeavesRowUnchanged`

### Requirement: item_attributes@v2 carries the listing status

The registry SHALL declare `item_attributes@v2`, per listing: `seller_id`, `category_id`, `price` (as v1) and `status`
(string, the `ListingStatus` enum name, for example `LISTING_STATUS_PUBLISHED`), computed as of `AS_OF` from the
point-in-time table `listings`, written to the offline snapshot and to versioned online keys with the manifest, freshness
and parity rules of the other views. `item_attributes@v1` SHALL stay byte-identical in definition and lock hash.
`status` SHALL be read from an optional `status` column of `listing_sellers.parquet`. WHEN the file has no `status`
column, THE SYSTEM SHALL write `status` as null for every listing, SHALL NOT fail the job, and SHALL log a warning naming
the column. `LISTING_STATUS_DELETED` is a known status and SHALL pass through unchanged. An unknown status is null, never a substituted constant.

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

#### Scenario: DELETED is a known status

- **WHEN** the export has a listing `L9` with `status=LISTING_STATUS_DELETED`
- **THEN** the `item_attributes@v2` row for `L9` has `status = LISTING_STATUS_DELETED` (not null) and the online key `fs:item_attributes:v2:L9` carries it
- **VERIFIED BY**: `platform-featurestore/tests/test_item_attributes.py` `test_v2_deleted_status_is_known`

#### Scenario: The v2 definition and lock are unchanged

- **WHEN** the registry drift check runs after this change
- **THEN** it exits 0 and the hash of `item_attributes@v2` equals the locked value
- **VERIFIED BY**: `platform-featurestore/tests/test_materialize.py` `test_committed_lock_matches_registry`

### Requirement: The warehouse exports listing attributes and marks deleted listings

team-analytics SHALL keep the `category_id`, `price` (minor units) and `status` of every listing it has seen on
`listing.events` next to its seller, the newest event winning.
The Parquet export SHALL write `listing_sellers.parquet` beside the other feature inputs on every export cycle (columns `listing_id`, `seller_id`,
`updated_at`, `category_id`, `price`, `status`), replaced atomically.
`status` SHALL be the `ListingStatus` enum name of `Listing.status` in the newest `ListingChanged` event (names defined in `platform/listing/v1/listing.proto`, for example `LISTING_STATUS_PUBLISHED`).
`status` SHALL be `LISTING_STATUS_DELETED` when the event has `change_type = CHANGE_TYPE_DELETED`, whatever `Listing.status` it carries.
`status` SHALL be null when it is `LISTING_STATUS_UNSPECIFIED` and the change type is not DELETED.
On equal `updated_at` the later-consumed event wins, except that `LISTING_STATUS_DELETED` is terminal.
Once a row has `LISTING_STATUS_DELETED`, no later-consumed non-DELETED event, including one with an equal or newer `updated_at`, SHALL change its `status`.
A deleted listing SHALL keep its row, with status `LISTING_STATUS_DELETED`.
Rows written before these columns existed, and rows of listings deleted before this change, SHALL have a null or stale `status` until refreshed by the listing replay command or by the listing's next event.
A change of `status` SHALL be visible in the parquet within 2 x `PARQUET_EXPORT_INTERVAL_SECONDS` of its consumption.

#### Scenario: A listing's category and price reach the listing export

- **WHEN** a seller creates a listing with a category and a price and the next export cycle completes
- **THEN** `listing_sellers.parquet` on the analytics volume has a row for that listing with that `category_id` and `price`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "A listing's category and price reach the listing export"

#### Scenario: An edited listing's attributes replace the old ones

- **WHEN** the seller changes that listing's price and the next export cycle completes
- **THEN** `listing_sellers.parquet` holds the new price for that listing and exactly one row for it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "An edited listing's attributes replace the old ones"

#### Scenario: Published status reaches the export

- **WHEN** a `ListingChanged` event for listing `L1` with `Listing.status = LISTING_STATUS_PUBLISHED` is consumed and the next export cycle completes
- **THEN** `listing_sellers.parquet` has exactly one row for `L1` whose `status` equals the string `LISTING_STATUS_PUBLISHED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_StatusIsEnumName`, `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_StoresStatus`, `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "Published status reaches the export"

#### Scenario: Every non-published status is exported by name

- **WHEN** events for listings with each of `LISTING_STATUS_DRAFT`, `_REJECTED`, `_PENDING_REVIEW`, `_BANNED` and `_HELD` are consumed and the export completes
- **THEN** each listing's `status` in `listing_sellers.parquet` equals its own enum name and none equals `LISTING_STATUS_PUBLISHED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_AllStatuses`, `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "Every non-published status is exported by name"

#### Scenario: A newer status replaces the older one

- **WHEN** listing `L1` is PUBLISHED at time T1 and a `ListingChanged` with `LISTING_STATUS_BANNED` at T2 > T1 is consumed and the export completes
- **THEN** the parquet has exactly one row for `L1` with `status = LISTING_STATUS_BANNED` and `updated_at = T2`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_NewerStatusWins`

#### Scenario: An older event does not regress the status

- **WHEN** an event for `L1` with `LISTING_STATUS_PUBLISHED` and `occurred_at` T1 arrives after the stored row has `LISTING_STATUS_BANNED` at T2 > T1
- **THEN** the stored and exported `status` for `L1` remains `LISTING_STATUS_BANNED`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_OlderStatusIgnored`

#### Scenario: A deleted listing is exported as DELETED

- **WHEN** a `ListingChanged` with `change_type = CHANGE_TYPE_DELETED` and `Listing.status = LISTING_STATUS_PUBLISHED` is consumed for `L1` and the next export cycle completes
- **THEN** `listing_sellers.parquet` has exactly one row for `L1`, with its `seller_id` and `status` equal to the string `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_DeletedIsDeletedStatus`, `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_DeletedKeepsRowWithDeletedStatus`, `platform-e2e/tests/e2e/features/featurestore/item_attributes.feature` "A deleted listing is exported as DELETED"

#### Scenario: DELETED wins over every carried status

- **WHEN** a `CHANGE_TYPE_DELETED` event is consumed whose `Listing.status` is, in turn, `LISTING_STATUS_UNSPECIFIED`, `LISTING_STATUS_BANNED` and the undefined number 99
- **THEN** each stored row has `status = LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_DeletedOverridesCarriedStatus`

#### Scenario: A change type other than DELETED never produces DELETED

- **WHEN** a `CREATED` or `UPDATED` event whose `Listing.status` is `LISTING_STATUS_PUBLISHED` is consumed
- **THEN** the stored `status` is `LISTING_STATUS_PUBLISHED`, not `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_OnlyDeletedChangeTypeIsDeleted`

#### Scenario: Equal timestamp: a non-DELETED event consumed after DELETED does not undo it

- **WHEN** the stored row of `L1` is `LISTING_STATUS_DELETED` at T2 and an event for `L1` with `LISTING_STATUS_PUBLISHED` and `occurred_at` equal to T2 is consumed afterwards
- **THEN** the stored and exported `status` of `L1` remains `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_EqualTimestampDoesNotUndoDeleted`

#### Scenario: A newer non-DELETED event does not undo DELETED

- **WHEN** the stored row of `L1` is `LISTING_STATUS_DELETED` at T2 and an event for `L1` with `LISTING_STATUS_PUBLISHED` and `occurred_at` T3 > T2 is consumed
- **THEN** the stored and exported `status` of `L1` remains `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_NewerNonDeletedEventDoesNotUndoDeleted`

#### Scenario: An older event does not undo DELETED

- **WHEN** the stored row of `L1` is `LISTING_STATUS_DELETED` at T2 and an event for `L1` with `LISTING_STATUS_PUBLISHED` and `occurred_at` T1 < T2 is consumed
- **THEN** the stored and exported `status` of `L1` remains `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_OlderEventDoesNotUndoDeleted`

#### Scenario: Equal timestamps let the later-consumed event win

- **WHEN** two events for `L1` with the same `occurred_at` and statuses `LISTING_STATUS_PUBLISHED` then `LISTING_STATUS_HELD` are consumed in that order
- **THEN** the stored `status` for `L1` is `LISTING_STATUS_HELD`
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_EqualTimestampLaterWins`

#### Scenario: Unspecified status is exported as null

- **WHEN** a `ListingChanged` with `Listing.status = LISTING_STATUS_UNSPECIFIED` is consumed for `L2` and the export completes
- **THEN** the row for `L2` has `status` null and is not an empty string
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_UnspecifiedIsNull`

#### Scenario: An unknown enum number is never written as a name or a number

- **WHEN** a `ListingChanged` carries a `Listing.status` number that is not defined in the proto enum (for example 99)
- **THEN** the row for that listing has `status` null
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_UnknownEnumIsNull`

#### Scenario: Existing rows get a null status after migration

- **WHEN** team-analytics starts against a warehouse whose `listing_sellers` table has no `status` column and holds 3 rows
- **THEN** the column exists, the 3 rows are kept with `status` null, and a second start exits without error and changes nothing
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestEnsureSchema_AddsListingStatusColumnIdempotently`

#### Scenario: Replaying the latest event backfills an existing row

- **WHEN** the latest `ListingChanged` of a listing with a null `status` (same `occurred_at` as the stored row) is consumed again and the export completes
- **THEN** the row's `status` equals that event's status name and the table still has one row for the listing
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_ReplayBackfillsStatus`

#### Scenario: The export file has the status column last

- **WHEN** an export cycle completes
- **THEN** the columns of `listing_sellers.parquet` are exactly `listing_id, seller_id, updated_at, category_id, price, status` in that order and the file is replaced atomically
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` `TestListingSellersParquetHasStatusColumn`

#### Scenario: Enum names come from the shared proto

- **WHEN** the unit test enumerates `listingv1.ListingStatus_name` from the generated code of `platform/listing/v1`
- **THEN** every value other than 0 is exported as the generated name when carried by a non-DELETED event and the set equals the names in `listing.proto`, including `LISTING_STATUS_DELETED`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_AllStatuses`

#### Scenario: A malformed event writes nothing

- **WHEN** a `listing.events` record is not a decodable envelope or `ListingChanged`
- **THEN** no row is written or changed and the record is handled by the existing error path (DLQ)
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_MalformedEvent`
