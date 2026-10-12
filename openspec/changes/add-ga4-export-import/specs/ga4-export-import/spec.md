## ADDED Requirements

### Requirement: The daily GA4 import builds ga4_sessions idempotently
The GA4 import job SHALL, for one export date, read the GA4 export `events_<date>` (BigQuery when `GA4_EXPORT_SOURCE=bigquery`, a Parquet fixture with the GA4 export schema when `fixture`) and replace all `ga4_sessions` rows with `export_date` = D (`DELETE WHERE export_date = D`, then insert) with one row per (`user_pseudo_id`, `session_id`) holding `export_date`, `source`, `medium`, `campaign`, `landing_page`, `ts` (first GA4 event of the session) and `ended_at` (last GA4 event ts of the session). The replace SHALL be atomic. `landing_page` SHALL contain no query string. When the source has no table for the date, the job SHALL fail without modifying `ga4_sessions`.

#### Scenario: Import of a fixture day produces sessions
- **WHEN** the job runs for a fixture date containing 3 sessions of 2 distinct `user_pseudo_id`s
- **THEN** `ga4_sessions` holds exactly 3 rows for that date, each with non-empty `source`, `medium` and `session_id`

#### Scenario: Import of 5M rows meets the time budget
- **WHEN** the job runs for a fixture date with 5,000,000 GA4 event rows
- **THEN** it finishes in 10 minutes or less and records an import run with that export date

#### Scenario: Re-running the same date is idempotent
- **WHEN** the job runs twice for the same export date
- **THEN** the `ga4_sessions` row count and content for that date are identical after both runs

#### Scenario: A missing export table fails without changing data
- **WHEN** the job runs for a date whose export table does not exist
- **THEN** the job exits with an error naming the date and the `ga4_sessions` rows of other dates are unchanged

#### Scenario: Landing page query strings are removed
- **WHEN** a fixture session has `page_location` `https://agora.test/p/1?email=a@b.com&utm_source=x`
- **THEN** the stored `landing_page` equals `https://agora.test/p/1`

#### Scenario: Empty day is a valid import
- **WHEN** the export table for a date exists with zero events
- **THEN** the job succeeds, writes zero rows for that date and records an import run with rows = 0

### Requirement: ga4_sessions is part of the parity-checked warehouse schema
`GA4SessionsSchema` SHALL be the single source of the `ga4_sessions` columns, including `export_date` (DATE, the key for the idempotent replace) and `ended_at` (TIMESTAMP, last GA4 event of the session); the DuckDB and BigQuery adapters SHALL derive their DDL from it and SHALL create or migrate the table with `ADD COLUMN IF NOT EXISTS`.

#### Scenario: Schema parity holds
- **WHEN** the parity test compares both adapters' `ga4_sessions` columns with `GA4SessionsSchema`
- **THEN** names, order and types match with zero differences

#### Scenario: ga4_sessions carries export_date
- **WHEN** the job imports dates D1 and D2 and then re-imports D1
- **THEN** every row has a non-null `export_date`, rows of D2 are unchanged and D1 rows equal the re-import result

#### Scenario: Migration is idempotent
- **WHEN** the adapter opens a database that already has `ga4_sessions` twice in a row
- **THEN** both opens succeed and the column list is unchanged

#### Scenario: A BigQuery migration failure is surfaced
- **WHEN** the BigQuery adapter's DDL call returns an error
- **THEN** adapter open fails with that error and no job run is recorded

### Requirement: tracking_events_attributed joins first-party events to GA4 sessions
The view `tracking_events_attributed` SHALL expose every row of `tracking_events_resolved` plus `source`, `medium`, `campaign` and `landing_page`, joined where `tracking_events.ga_client_id = ga4_sessions.user_pseudo_id` (`ga_client_id` stored without the `GA1.1.` prefix) and the event time falls in the session. A session window is [`ts`, min(next session `ts` of the same `user_pseudo_id`, `ended_at` + 30 min)). Events without a match, or with empty `ga_client_id`, SHALL be kept with `source`='(none)'. The view SHALL NOT multiply rows when a client has several sessions.

#### Scenario: An event with a matching client id is attributed
- **WHEN** a tracked view carries `ga_client_id` `111.222` and `ga4_sessions` has a session for `111.222` with source `google`, medium `cpc`
- **THEN** that event row in `tracking_events_attributed` has source `google` and medium `cpc`

#### Scenario: An event outside any session is unattributed
- **WHEN** a tracked event has a matching `ga_client_id` but occurs 2 hours after the end of that client's last session
- **THEN** the view returns that event exactly once with source `(none)`

#### Scenario: Session end is bounded by the next session or 30 minutes after ended_at
- **WHEN** a client has session A (`ts` 10:00, `ended_at` 10:15) and session B (`ts` 10:20, `ended_at` 10:30), and events at 10:18, 10:25 and 11:30
- **THEN** the 10:18 event is attributed to A (window 10:00 to 10:20), the 10:25 event to B (window 10:20 to 11:00) and the 11:30 event has source `(none)`

#### Scenario: ga4_sessions stores ended_at
- **WHEN** a fixture session has GA4 events at 10:00, 10:07 and 10:15
- **THEN** its row has `ts` 10:00 and `ended_at` 10:15

#### Scenario: An event without a client id is kept as unattributed
- **WHEN** a tracked view has an empty `ga_client_id`
- **THEN** the view returns that event exactly once with source `(none)`

#### Scenario: A client with two sessions does not duplicate events
- **WHEN** `ga4_sessions` holds two sessions for one client id and the client has 5 events
- **THEN** the view returns exactly 5 rows for that client

### Requirement: Ops can read the funnel split by acquisition channel
`AnalyticsQueryService.GetChannelFunnel` (admin-only, provided by `ga4-consent-contract`; request: `from`, `to` (Timestamp) + optional `seller_id`; response `repeated ChannelFunnelRow rows` of `{source, medium, campaign, impressions, views, adds, begin_checkouts, purchases}` int64) SHALL return impressions, views, adds, begin_checkouts and purchases (distinct `transaction_id`) per (`source`, `medium`, `campaign`) over a window, computed from `tracking_events_attributed`. Orders and revenue figures, if shown, SHALL come only from `order_facts`; no query SHALL sum revenue from GA4 data. It SHALL require the `admin` scope; the gateway SHALL gate it at the edge. Over a 30-day window it SHALL answer with p95 latency under 2 s.

#### Scenario: A seller filter narrows the funnel
- **WHEN** an admin requests the channel funnel with `seller_id` = S for events of sellers S and T
- **THEN** every returned count includes only events of seller S

#### Scenario: Counts are split by channel
- **WHEN** an admin requests the channel funnel for seeded events of two campaigns, 4 views for `spring` and 2 for `summer`
- **THEN** `rows` has a `spring` ChannelFunnelRow with `views` = 4 and a `summer` row with `views` = 2

#### Scenario: Funnel query meets p95 latency
- **WHEN** an admin calls `GetChannelFunnel` 100 times with `from`/`to` spanning a 30-day window seeded with 1,000,000 `tracking_events`
- **THEN** the p95 latency is under 2 seconds

#### Scenario: A window with no events returns no rows
- **WHEN** an admin requests the channel funnel for a window with no events
- **THEN** `rows` is empty and the call succeeds

#### Scenario: An invalid window is rejected
- **WHEN** an admin requests the channel funnel with `from` later than `to`
- **THEN** the call fails with `invalid_argument`

#### Scenario: Only admins can read the channel funnel
- **WHEN** an anonymous client and then a logged-in buyer request the channel funnel through the gateway
- **THEN** the gateway answers HTTP 401 and then HTTP 403, and a direct call without scope `admin` returns PERMISSION_DENIED with no rows

#### Scenario: Revenue is never read from GA4 data
- **WHEN** `ga4_sessions` and `ga4_events_daily` hold purchase values and `order_facts` holds a different total
- **THEN** every revenue query returns the `order_facts` total

### Requirement: The import is deployed with disabled-by-default configuration
`platform-gitops` SHALL define a daily CronJob for the import and the settings `GA4_EXPORT_SOURCE`, `GA4_EXPORT_DATASET` and `GA4_RECONCILE_MAX_DIFF_RATIO`; when `GA4_EXPORT_DATASET` is unset in `local` and `staging`, the job SHALL exit 0 without reading anything.

#### Scenario: The job is a no-op when unconfigured
- **WHEN** the job starts with `GA4_EXPORT_DATASET` unset
- **THEN** it exits with status 0 and writes zero rows

#### Scenario: Invalid threshold is rejected at startup
- **WHEN** the job starts with `GA4_RECONCILE_MAX_DIFF_RATIO=-1`
- **THEN** it exits with a non-zero status and an error naming the setting
