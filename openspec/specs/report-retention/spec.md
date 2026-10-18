# report-retention Specification

## Purpose
Closed listing reports are pseudonymised after two years.

## Requirements

### Requirement: Closed reports are pseudonymised after 2 years

A background sweep in team-engagement SHALL, for each report with status `ACTIONED` or `DISMISSED` whose `resolved_at` is older than `REPORT_RETENTION_DAYS` (default 730), set `reporter_id` to the literal `redacted` and `note` to the empty string, in batches of at most 500 rows. It SHALL never touch an `OPEN` report, a report resolved within the window, or any other column, and SHALL run every `REPORT_SWEEP_INTERVAL` (default 24h) and once at startup.

#### Scenario: Listing reports: Old closed report is pseudonymised
- **WHEN** the sweep runs with a report closed 731 days ago with reporter "u-1" and note "my phone 0900"
- **THEN** that row has `reporter_id` `redacted`, `note` "" and unchanged `status`, `target_id`, `reason_code`, `resolution_note` and `resolved_at`
- **VERIFIED BY**: team-engagement `internal/repository` `TestSweepPseudonymisesOldClosedReports`

#### Scenario: Listing reports: Retention boundary
- **WHEN** the sweep runs with reports closed exactly 730 days ago minus 1 minute and 730 days ago plus 1 minute
- **THEN** the first is untouched and the second is pseudonymised
- **VERIFIED BY**: team-engagement `internal/repository` `TestSweepRetentionBoundary`

#### Scenario: Listing reports: Open reports are never swept
- **WHEN** the sweep runs with an `OPEN` report created 800 days ago
- **THEN** the report keeps its `reporter_id` and `note`
- **VERIFIED BY**: team-engagement `internal/repository` `TestSweepSkipsOpenReports`

#### Scenario: Listing reports: Sweep is idempotent and batched
- **WHEN** the sweep runs twice over 1,200 eligible rows
- **THEN** the first run updates 1,200 rows in 3 batches of at most 500, the second updates 0, and a failure in batch 2 leaves batch 1 committed and the next run finishes the rest
- **VERIFIED BY**: team-engagement `internal/repository` `TestSweepIdempotentAndBatched`

#### Scenario: Listing reports: Pseudonymised reporter can report again
- **WHEN** a user's old closed report was pseudonymised and the user reports the same listing
- **THEN** a new `OPEN` report is stored with the user's real id
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportAfterSweep`

#### Scenario: Listing reports: Retention config is validated
- **WHEN** the service starts with `REPORT_RETENTION_DAYS` of 0 or a negative number, or `REPORT_SWEEP_INTERVAL` of 0
- **THEN** config validation fails with an error naming the variable
- **VERIFIED BY**: team-engagement `internal/config` `TestReportRetentionConfigValidated`

### Requirement: Reports schema migration

Migration `0012_reports` SHALL create the `reports` table and its indexes, and its down migration SHALL drop them. The partial unique index SHALL cover only `OPEN` rows.

#### Scenario: Listing reports: Migration creates the schema
- **WHEN** the up migration runs on a database at the previous version
- **THEN** table `reports` exists with the unique partial index on `(reporter_id, target_type, target_id) WHERE status = 'OPEN'`
- **VERIFIED BY**: team-engagement `internal/repository` `TestMigrationReportsUp`

#### Scenario: Listing reports: Down migration removes the schema
- **WHEN** the down migration runs
- **THEN** table `reports` and its indexes no longer exist and other tables are unchanged
- **VERIFIED BY**: team-engagement `internal/repository` `TestMigrationReportsDown`

#### Scenario: Listing reports: In-memory repository matches Postgres
- **WHEN** the same create, duplicate, close and list sequence runs against the in-memory and Postgres repositories
- **THEN** both give identical results
- **VERIFIED BY**: team-engagement `internal/repository` `TestReportRepositoryParity`
