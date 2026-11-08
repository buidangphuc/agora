## MODIFIED Requirements

### Requirement: The warehouse exports listing attributes

team-analytics SHALL keep the `category_id`, `price` (minor units) and `status` of every listing it has seen on
`listing.events` next to its seller, the newest event winning, and the Parquet export SHALL write
`listing_sellers.parquet` beside the other feature inputs on every export cycle (columns `listing_id`, `seller_id`,
`updated_at`, `category_id`, `price`, `status`), replaced atomically. `status` SHALL be the `ListingStatus` enum name of
`Listing.status` in the newest `ListingChanged` event (names defined in `platform/listing/v1/listing.proto`, for example
`LISTING_STATUS_PUBLISHED`), and SHALL be null when the status is `LISTING_STATUS_UNSPECIFIED` or when the newest event
has `change_type = CHANGE_TYPE_DELETED`. On equal `updated_at` the later-consumed event wins. A deleted listing SHALL keep its row.
Rows written before these columns existed SHALL have a null `status` until refreshed by the listing replay command or by
the listing's next event. A change of `status` SHALL be visible in the parquet within 2 x `PARQUET_EXPORT_INTERVAL_SECONDS`
of its consumption.

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

#### Scenario: A deleted listing keeps its row with a null status

- **WHEN** a `ListingChanged` with `change_type = CHANGE_TYPE_DELETED` and `Listing.status = LISTING_STATUS_PUBLISHED` is consumed for `L1`
- **THEN** the stored row for `L1` still exists with its `seller_id` and `status` null
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_DeletedHasNullStatus`, `team-analytics/internal/warehouse/duckdb/duckdb_test.go` `TestUpsertListingSellers_DeletedKeepsRowNullStatus`

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
- **THEN** every value other than 0 is exported as the generated name and the set equals the names in `listing.proto`
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_AllStatuses`

#### Scenario: A malformed event writes nothing

- **WHEN** a `listing.events` record is not a decodable envelope or `ListingChanged`
- **THEN** no row is written or changed and the record is handled by the existing error path (DLQ)
- **VERIFIED BY**: `team-analytics/internal/events/listing_test.go` `TestListingSellerFromEnvelope_MalformedEvent`

## ADDED Requirements

### Requirement: A one-off replay command backfills listing status

team-analytics SHALL provide the command `listing-replay` (`cmd/listing-replay`) that reads `KAFKA_LISTING_TOPIC` from the
earliest offset to the end offsets captured at start, on its own consumer group `KAFKA_LISTING_REPLAY_GROUP` that is
different from `KAFKA_LISTING_CONSUMER_GROUP`, upserts every decodable `ListingChanged` through the normal decoder and
upsert, and exits 0 when done. It SHALL NOT be started by the service and SHALL NOT be an enable flag. It SHALL NOT move
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
