## MODIFIED Requirements

### Requirement: The daily GA4 import builds ga4_sessions idempotently
The GA4 import job SHALL, for one export date, read the GA4 export `events_<date>` (BigQuery from `GA4_EXPORT_DATASET`, or, when `MOCK_DATA` names `ga4export`, a Parquet fixture with the GA4 export schema from `GA4_EXPORT_FIXTURE_DIR`) and replace all `ga4_sessions` rows with `export_date` = D (`DELETE WHERE export_date = D`, then insert) with one row per (`user_pseudo_id`, `session_id`) holding `export_date`, `source`, `medium`, `campaign`, `landing_page`, `ts` (first GA4 event of the session) and `ended_at` (last GA4 event ts of the session). The replace SHALL be atomic. `landing_page` SHALL contain no query string. When the source has no table for the date, the job SHALL fail without modifying `ga4_sessions`.

#### Scenario: Import of a fixture day produces sessions
- **WHEN** the job runs with `MOCK_DATA=ga4export` for a fixture date containing 3 sessions of 2 distinct `user_pseudo_id`s
- **THEN** `ga4_sessions` holds exactly 3 rows for that date, each with non-empty `source`, `medium` and `session_id`
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestImportOfAFixtureDayProducesSessions`

#### Scenario: Import of 5M rows meets the time budget
- **WHEN** the job runs for a fixture date with 5,000,000 GA4 event rows
- **THEN** it finishes in 10 minutes or less and records an import run with that export date
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestImportOf5MRowsMeetsTheTimeBudget`

#### Scenario: Re-running the same date is idempotent
- **WHEN** the job runs twice for the same export date
- **THEN** the `ga4_sessions` row count and content for that date are identical after both runs
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestReRunningTheSameDateIsIdempotent`

#### Scenario: A missing export table fails without changing data
- **WHEN** the job runs for a date whose export table does not exist
- **THEN** the job exits with an error naming the date and the `ga4_sessions` rows of other dates are unchanged
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestMissingExportTableFailsWithoutChangingData`

#### Scenario: Landing page query strings are removed
- **WHEN** a fixture session has `page_location` `https://agora.test/p/1?email=a@b.com&utm_source=x`
- **THEN** the stored `landing_page` equals `https://agora.test/p/1`
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestLandingPageQueryStringsAreRemoved`

#### Scenario: Empty day is a valid import
- **WHEN** the export table for a date exists with zero events
- **THEN** the job succeeds, writes zero rows for that date and records an import run with rows = 0
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestEmptyDayIsAValidImport`

### Requirement: The import runs inside the consumer process
The analytics consumer process SHALL run the GA4 import once per day for the previous UTC date (time configurable, `GA4_IMPORT_AT`, default `03:30` UTC) using its already-open warehouse writer (DuckDB or BigQuery), so no second process writes to the warehouse. It SHALL expose an internal, cluster-only trigger `POST /internal/ga4import?date=YYYY-MM-DD` on the consumer's internal HTTP listener (`METRICS_ADDR`, `:9109`; not routed through the gateway) that runs the import synchronously and returns 200 with the row count, 4xx for an invalid date, and 5xx with the date in the body on failure (for example a missing export table). The CLI `/ga4import --date` SHALL be a thin client of that trigger at `GA4_IMPORT_TRIGGER_URL`: exit 0 on 200, non-zero with the error text on stderr otherwise. When `GA4_EXPORT_DATASET` is empty and `MOCK_DATA` does not name `ga4export`, the scheduled run and the trigger SHALL be no-ops returning 200 with rows = 0.

#### Scenario: The import runs inside the consumer without a second writer
- **WHEN** the consumer holds the warehouse writer and `POST /internal/ga4import?date=D` is called for a fixture date with `MOCK_DATA=ga4export`
- **THEN** the response is 200 with the row count, `ga4_sessions` holds the rows for D, and no second warehouse connection is opened
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestTriggerImportsFixtureDateThroughTheConsumerWriter_TheImportRunsInsideTheConsumerWithoutASecondWriter`

#### Scenario: The internal trigger is not exposed through the gateway
- **WHEN** the gateway route table and the team-analytics gitops manifests are inspected
- **THEN** no gateway route maps to `/internal/ga4import` and the metrics/trigger port has no Ingress or public Service exposure
- **VERIFIED BY**: platform-e2e/scripts/tests/test_gitops_ga4.py

#### Scenario: An invalid date is rejected by the trigger
- **WHEN** `POST /internal/ga4import?date=not-a-date` is called
- **THEN** the response is a 4xx and `ga4_sessions` is unchanged
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestTriggerRejectsInvalidDateWith4xx_AnInvalidDateIsRejectedByTheTrigger`

#### Scenario: A failed import returns 5xx with the date
- **WHEN** the trigger is called for a date whose export table does not exist
- **THEN** the response is 5xx with the date in the body and the CLI exits non-zero with that text on stderr
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestTriggerReturns5xxWithDateWhenExportTableMissing_AFailedImportReturns5xxWithTheDate`

#### Scenario: The CLI reads the trigger address from settings
- **WHEN** the CLI runs with `GA4_IMPORT_TRIGGER_URL` set to a stub that answers 200
- **THEN** the stub receives one POST and the CLI exits 0
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestCLICallsTriggerAndExitsZeroOn200_TheImportRunsInsideTheConsumer`

### Requirement: The import is deployed with disabled-by-default configuration
`platform-gitops` SHALL set `GA4_EXPORT_DATASET`, `GA4_IMPORT_AT` and `GA4_RECONCILE_MAX_DIFF_RATIO` on the team-analytics consumer Deployment (no CronJob), SHALL NOT set `MOCK_DATA` there, and SHALL keep the metrics/trigger port cluster-internal; when `GA4_EXPORT_DATASET` is unset in `local` or `test` and `MOCK_DATA` does not name `ga4export`, the import SHALL return success with rows = 0 without reading anything, in `staging` and `prod` an unset `GA4_EXPORT_DATASET` SHALL fail `config.Load`, and it SHALL never read fixtures there because `MOCK_DATA` is refused; when `GA4_EXPORT_DATASET` is set and `MOCK_DATA` does not name `ga4export`, the import SHALL query BigQuery in every `ENV` and a missing table fails the run; when `MOCK_DATA` names `ga4export` (local or test only) the fixture source is used even if `GA4_EXPORT_DATASET` is set.

#### Scenario: The job is a no-op when unconfigured
- **WHEN** the scheduled run or the trigger executes with `ENV=local`, `GA4_EXPORT_DATASET` unset and `MOCK_DATA` unset
- **THEN** it returns 200 (CLI exit 0) with rows = 0 and writes zero rows
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestTriggerIsANoOpWhenUnconfigured_TheJobIsANoOpWhenUnconfigured`

#### Scenario: Invalid threshold is rejected at startup
- **WHEN** the consumer or the CLI starts with `GA4_RECONCILE_MAX_DIFF_RATIO=-1`
- **THEN** it exits with a non-zero status and an error naming the setting
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestInvalidThresholdIsRejectedAtStartup`

#### Scenario: The deployed manifest does not set mock data
- **WHEN** `platform-gitops/platform/team-analytics/consumer.yaml` is parsed
- **THEN** no container env entry is named `MOCK_DATA` or `GA4_EXPORT_SOURCE`
- **VERIFIED BY**: platform-e2e/scripts/tests/test_gitops_ga4.py
