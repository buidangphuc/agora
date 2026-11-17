## ADDED Requirements

### Requirement: team-analytics consumes UserErased with its own group and DLQ

The team-analytics worker SHALL consume `identity.events` (`IDENTITY_EVENTS_TOPIC`) in its own consumer group `ERASURE_CONSUMER_GROUP` and act only on envelopes of type `platform.identity.v1.UserErased`. Offsets SHALL be committed only after the purge transaction committed. An undecodable envelope or a `UserErased` with an empty `user_id` SHALL be published to `ERASURE_DLQ_TOPIC` (`identity.events.analytics.dlq`) and then committed past. Envelopes of any other type SHALL be skipped without a DLQ record.

#### Scenario: AUE-1 A UserErased event purges the user
- **WHEN** a `UserErased` for user U is produced to `identity.events` while U has rows in `tracking_events`, `order_facts` and `engagement_facts`
- **THEN** within 60 s no row in any team-analytics table contains U's id in any column and the consumer group offset has advanced past the record
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-1 A UserErased event purges the user`

#### Scenario: AUE-2 Another identity event type is skipped
- **WHEN** a `UserStatusChanged` envelope is produced to `identity.events`
- **THEN** no table changes, the DLQ topic receives no record and the offset advances
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-2 Another identity event type is skipped`

#### Scenario: AUE-3 Poison record goes to the DLQ
- **WHEN** a record with bytes that are not a valid `EventEnvelope`, and a `UserErased` with an empty `user_id`, are produced
- **THEN** each is published to `identity.events.analytics.dlq` with an `error` header, no table changes, and the offset advances past both
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-3 Poison record goes to the DLQ`

#### Scenario: AUE-4 A failed purge is retried and commits nothing
- **WHEN** the DuckDB transaction fails while purging user U
- **THEN** the offset is not committed, the transaction leaves no partial change (all U rows keep their original id), and the redelivery succeeds once the database accepts writes
- **VERIFIED BY**: `team-analytics/internal/events/erasure_test.go` `TestErasureConsumerPurgeFailureRetriesWithoutCommit`

#### Scenario: AUE-5 Redelivery is a no-op
- **WHEN** the same `UserErased` for U is delivered a second time after a successful purge
- **THEN** no row changes, the tombstone count for U stays 1, no DLQ record is written and the offset advances
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-5 Redelivery is a no-op`

### Requirement: Tracking rows of the erased user are re-keyed to a random pseudonym

For every `tracking_events` row with `principal_type = 'user'` and `principal_id` equal to the erased user, the purge SHALL set `principal_id`, `session_id` and `anonymous_id` to one pseudonym `erased:<32 hex>` drawn from `crypto/rand`, and SHALL set `ga_client_id` and `properties` to NULL. Every other row whose `anonymous_id` equals an `anonymous_id` of those rows SHALL get `anonymous_id` = the pseudonym and `ga_client_id` = NULL. Row count, `event_type`, `listing_id`, timestamps and commerce fields SHALL be unchanged.

#### Scenario: AUE-6 Re-key keeps aggregates and removes identifiers
- **WHEN** U (50 rows, two anonymous ids A1 and A2, each also used by 10 pre-login anonymous rows) is erased
- **THEN** `tracking_events` still has the same total row count, zero rows contain U's id, A1 or A2 in any column, all 70 rows carry the same single `erased:` pseudonym, and `GetSellerFunnel` counts for the affected listings are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-6 Re-key keeps aggregates and removes identifiers`

#### Scenario: AUE-7 Rows of other users are untouched
- **WHEN** user U is erased and user V has 20 tracking rows
- **THEN** V's 20 rows are byte-identical before and after, including `principal_id`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-7 Rows of other users are untouched`

#### Scenario: AUE-40 A service principal row with the same id is not re-keyed
- **WHEN** U is erased and a `tracking_events` row has `principal_type = 'service'` with `principal_id` equal to U's id
- **THEN** that row is not changed, because only rows of type `user` are re-keyed, and the purge still commits
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/erase_test.go` `TestErasureLeavesServicePrincipalRows`

#### Scenario: AUE-8 Boundary of a user with no rows
- **WHEN** a `UserErased` arrives for a user with zero rows in every table
- **THEN** the consumer commits the offset, writes one tombstone, changes no other row and writes no DLQ record
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-8 Boundary of a user with no rows`

### Requirement: Order, engagement and listing rows are pseudonymised or deleted

The purge SHALL re-key `order_facts.buyer_id`, `order_facts.seller_id`, `engagement_facts.seller_id` and `listing_sellers.seller_id` equal to the user to the same pseudonym, SHALL re-key `engagement_facts.user_id` of `review_created` facts to the pseudonym, and SHALL delete `engagement_facts` rows of the user with fact `favorite_added`, `favorite_removed`, `seller_followed` or `seller_unfollowed`. The purge SHALL scrub personal identifiers only: `order_id`, quantities, prices and timestamps SHALL be unchanged, `listing_sellers` rows SHALL be kept, and the purge SHALL NOT change any `status` value.

#### Scenario: AUE-9 Orders are kept with a pseudonymous buyer
- **WHEN** buyer U with 3 PAID `order_facts` lines is erased
- **THEN** the 3 lines remain with identical `order_id`, `quantity`, `unit_price` and `occurred_at`, `buyer_id` is the pseudonym, and `GetPlatformOrderSummary` GMV over the same window is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-9 Orders are kept with a pseudonymous buyer`

#### Scenario: AUE-10 Favourites and follows are deleted
- **WHEN** U with 4 favourite facts and 2 follow facts is erased
- **THEN** `engagement_facts` has no row with `user_id` = U for those facts, `favorites_current` and `follows_current` have no row for U, and V's facts are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-10 Favourites and follows are deleted`

#### Scenario: AUE-11 An erased seller is re-keyed everywhere
- **WHEN** seller U whose `roles` include `seller`, and whose listings are recorded in `listing_sellers` just before the erasure, is erased
- **THEN** no row in `order_facts`, `engagement_facts` or `listing_sellers` has `seller_id` = U, each recorded listing has the same `listing_id` and the same `status` after the erasure as before it (a listing that is `LISTING_STATUS_DELETED` before stays `LISTING_STATUS_DELETED`), and `GetSellerFunnel` for U returns zero counts
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-11 An erased seller is re-keyed everywhere`

#### Scenario: AUE-35 A failed order or engagement re-key rolls back as a whole
- **WHEN** the re-key of `engagement_facts` fails after `order_facts` was re-keyed within U's purge
- **THEN** the transaction is rolled back, U's `order_facts` rows keep the original `buyer_id`, no tombstone exists and the record is retried without a DLQ record
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/erase_test.go` `TestErasureRollsBackWhenEngagementRekeyFails`

#### Scenario: AUE-36 A forged event for another user does not touch unrelated rows
- **WHEN** a `UserErased` names a user id that is a strict prefix or suffix of V's id
- **THEN** no row of V changes, because matching is by exact equality on the whole column value
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-36 A forged event for another user does not touch unrelated rows`

### Requirement: Late events of an erased user are re-keyed on ingest

The purge SHALL write a tombstone `erased_users(user_hmac, pseudonym, erased_at)` where `user_hmac = HMAC-SHA256(user_id, ERASURE_TOMBSTONE_SECRET)`, in the same transaction as the re-key. The `tracking_events`, `order_facts` and `engagement_facts` writers SHALL, inside the insert transaction, replace the user id of any row whose HMAC is in the tombstone with that pseudonym. The tombstone SHALL NOT hold the user id.

#### Scenario: AUE-12 A buffered event flushed after the purge is re-keyed
- **WHEN** a tracking event for U is in the writer's in-memory batch when U's purge commits, and the batch flushes afterwards
- **THEN** the stored row carries U's pseudonym, and no table contains U's id
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-12 A buffered event flushed after the purge is re-keyed`

#### Scenario: AUE-13 The tombstone holds no clear id
- **WHEN** an operator reads every column of `erased_users` after U's erasure
- **THEN** none equals U's id and `user_hmac` is 64 hex characters that differ when `ERASURE_TOMBSTONE_SECRET` differs
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-13 The tombstone holds no clear id`

#### Scenario: AUE-37 A late event for a non-erased user is stored as is
- **WHEN** a tracking event for V, whose HMAC is not in `erased_users`, is written
- **THEN** the row keeps V's id and the tombstone table is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-37 A late event for a non-erased user is stored as is`

#### Scenario: AUE-38 Tombstone lookup failure rejects the batch
- **WHEN** the tombstone lookup fails inside the insert transaction
- **THEN** the batch is rolled back with no rows written, offsets are not committed and the batch is retried
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/erase_test.go` `TestErasureWriterTombstoneLookupFailureRollsBackBatch`

### Requirement: Parquet exports and logs carry no erased id

The Parquet export SHALL contain no erased user id in any of its files within one `PARQUET_EXPORT_INTERVAL_SECONDS` after the purge commits. Logs, metrics labels and DLQ headers written by the erasure consumer SHALL NOT contain the raw user id; they SHALL carry the `kafkax` key fingerprint and row counts only.

#### Scenario: AUE-14 Exported files are clean after one cycle
- **WHEN** one export cycle completes after U's purge
- **THEN** a scan of the seven Parquet files for U's id, A1 and A2 finds zero matches
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure.feature` `AUE-14 Exported files are clean after one cycle`

#### Scenario: AUE-15 Logs carry no raw id
- **WHEN** U's erasure is processed with `LOG_LEVEL=debug`
- **THEN** the captured log output contains zero occurrences of U's id and includes the fingerprint and per-table counts
- **VERIFIED BY**: `team-analytics/internal/events/erasure_test.go` `TestErasureLogsCarryNoRawID`

#### Scenario: AUE-39 A failed export keeps the previous clean file and never exposes a partial one
- **WHEN** an export of one file fails after U's purge
- **THEN** the other files are still rewritten without U's id, the failed file is reported in logs and the next successful cycle replaces it without U's id
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` `TestExportFailedFileKeepsPreviousCleanFile`

### Requirement: Erasure configuration is validated at boot

The service SHALL read `IDENTITY_EVENTS_TOPIC`, `ERASURE_CONSUMER_GROUP`, `ERASURE_DLQ_TOPIC` and `ERASURE_TOMBSTONE_SECRET`, SHALL refuse to boot when any is empty, SHALL require `ERASURE_TOMBSTONE_SECRET` explicitly when ENV is `staging` or `prod`, and `.env.example` SHALL list exactly the keys of the config struct.

#### Scenario: AUE-16 Empty erasure setting refuses boot
- **WHEN** the worker boots and `ERASURE_CONSUMER_GROUP` is empty
- **THEN** config loading fails with an error naming `ERASURE_CONSUMER_GROUP` and the process exits non-zero
- **VERIFIED BY**: `team-analytics/internal/config/config_test.go` `TestLoadRejectsEmptyErasureSettings`

#### Scenario: AUE-17 Strict env needs the tombstone secret
- **WHEN** the worker boots with ENV `prod` and `ERASURE_TOMBSTONE_SECRET` unset
- **THEN** config loading fails naming `ERASURE_TOMBSTONE_SECRET`
- **VERIFIED BY**: `team-analytics/internal/config/config_test.go` `TestLoadStrictEnvRequiresTombstoneSecret`

#### Scenario: AUE-18 Env example stays in sync
- **WHEN** `make check-env` runs after the four keys are added to config and `.env.example`
- **THEN** `TestEnvExampleInSync` passes
- **VERIFIED BY**: `team-analytics/internal/config/config_test.go` `TestErasureEnvExampleListsTheFourKeys`
