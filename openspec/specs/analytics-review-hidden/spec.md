# analytics-review-hidden Specification

## Purpose
team-analytics records hidden reviews as review_hidden facts and stores review_id on review facts, so downstream counts can exclude hidden reviews.

## Requirements

### Requirement: The engagement consumer records a review_hidden fact

The engagement consumer SHALL map an `engagement.events` envelope of type `platform.engagement.v1.ReviewHidden` to one `engagement_facts` row with `fact = review_hidden`, the payload `review_id` and `listing_id`, empty `user_id` and `seller_id`, NULL `rating`, `event_id` equal to the envelope `event_id`, and `occurred_at` equal to the payload `hidden_at` (the envelope `occurred_at` when `hidden_at` is unset). The original `review_created` row SHALL be left untouched.

#### Scenario: Analytics review hidden: a ReviewHidden envelope maps to a review_hidden fact
- **WHEN** an envelope of type `platform.engagement.v1.ReviewHidden` with review id "r-1", listing id "l-1" and `hidden_at` 2026-10-18T10:00:00Z is mapped
- **THEN** the record has `Fact` "review_hidden", `ReviewID` "r-1", `ListingID` "l-1", empty `UserID` and `SellerID`, `Rating` 0 and `OccurredAt` 2026-10-18T10:00:00Z, and no error is returned
- **VERIFIED BY**: team-analytics `internal/consumer` `TestEngagementFactFromEnvelope`

#### Scenario: Analytics review hidden: a produced ReviewHidden reaches the warehouse
- **WHEN** a ReviewCreated and then a ReviewHidden envelope for the same review, each with a unique event id, are produced to engagement.events
- **THEN** within 30 seconds the warehouse holds one `review_created` row and one `review_hidden` row (with that review id and listing id, NULL rating), and the DLQ topic holds no record for either event id
- **VERIFIED BY**: platform-e2e `features/analytics/review_hidden.feature` "Analytics review hidden: a produced ReviewHidden reaches the warehouse"

#### Scenario: Analytics review hidden: the review_created row is kept
- **WHEN** a `review_hidden` fact is written for a review that already has a `review_created` row
- **THEN** the `review_created` row still exists unchanged (rating 4 stays 4) and the table holds two rows for the pair
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewHiddenKeepsReviewCreated`

#### Scenario: Analytics review hidden: hidden_at missing falls back to the envelope time
- **WHEN** a ReviewHidden envelope with no `hidden_at` and envelope `occurred_at` 2026-10-18T09:00:00Z is mapped
- **THEN** the record `OccurredAt` equals 2026-10-18T09:00:00Z
- **VERIFIED BY**: team-analytics `internal/consumer` `TestReviewHiddenOccurredAtFallback`

#### Scenario: Analytics review hidden: a replayed event is stored once
- **WHEN** the same ReviewHidden envelope (same event id) is delivered twice
- **THEN** the warehouse holds exactly one `review_hidden` row for that event id
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewHiddenIdempotent`

#### Scenario: Analytics review hidden: a corrupt payload is dead-lettered
- **WHEN** a ReviewHidden envelope whose payload bytes are not valid protobuf, or which has no event id, is processed in a batch with good records
- **THEN** the mapping returns an error, the record is published to the DLQ with an `error` header, and the other records of the batch are still written
- **VERIFIED BY**: team-analytics `internal/consumer` `TestEngagementFactFromEnvelopeRejects`
- **VERIFIED BY**: team-analytics `internal/consumer` `TestProcessEngagementRecordsDeadLettersAndContinues`

#### Scenario: Analytics review hidden: existing engagement types are unchanged
- **WHEN** envelopes of the five existing engagement types and an unknown type "platform.engagement.v1.Nope" are mapped
- **THEN** the five map to the same facts as before this change and the unknown type still returns `ErrUnknownEngagementType`
- **VERIFIED BY**: team-analytics `internal/consumer` `TestEngagementFactFromEnvelope`
- **VERIFIED BY**: team-analytics `internal/consumer` `TestEngagementFactFromEnvelopeRejects`

### Requirement: review_created rows persist review_id

The engagement consumer SHALL store `ReviewCreated.review_id` in `engagement_facts.review_id` for `review_created` rows written from now on; rows written before this change keep NULL.

#### Scenario: Analytics review hidden: a review_created row stores its review id
- **WHEN** a ReviewCreated envelope with review id "r-1", listing id "l-1" and rating 4 is mapped and written
- **THEN** the record has `ReviewID` "r-1" and the stored row has `review_id` "r-1", rating 4 and the same listing id as the later `review_hidden` row for "r-1"
- **VERIFIED BY**: team-analytics `internal/consumer` `TestEngagementFactFromEnvelope`
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewIDStoredForCreatedAndHidden`

#### Scenario: Analytics review hidden: a legacy review_created row is not given a review id without a backfill
- **WHEN** a database holding a review_created row written before the column existed is opened and a `review_hidden` row for another review is written
- **THEN** the legacy row still has NULL `review_id` and rating intact, and no backfill occurs
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestOpenAddsReviewIDColumnIdempotently`

### Requirement: The warehouse schema and Parquet export carry review_hidden

The DuckDB `engagement_facts` table SHALL have a nullable `review_id` column added without data loss to an existing database, populated for `review_created` and `review_hidden` rows, and `engagement_facts.parquet` SHALL include `review_hidden` rows and the `review_id` column.

#### Scenario: Analytics review hidden: opening an existing database adds review_id once
- **WHEN** a DuckDB file created before this change (no `review_id` column, with existing rows) is opened twice
- **THEN** both opens succeed, the column exists once, and the pre-existing rows are intact with NULL `review_id`
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestOpenAddsReviewIDColumnIdempotently`

#### Scenario: Analytics review hidden: the Parquet export includes hidden rows
- **WHEN** the export runs after a `review_hidden` row was written
- **THEN** `engagement_facts.parquet` is readable and contains a row with `fact` "review_hidden" and its `review_id`, and the file count stays seven
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestExportRelationWritesReadableParquetForTablesAndViews`

#### Scenario: Analytics review hidden: the favorites and follows views ignore review_hidden
- **WHEN** a `review_hidden` row exists beside favorite and follow facts
- **THEN** `favorites_current` and `follows_current` return the same rows as without it
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewHiddenDoesNotAffectCurrentViews`
