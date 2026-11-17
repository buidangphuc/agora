# analytics-experiment-reports Specification

## Purpose
Defines the admin-only retention-cohort, experiment-readout and AI-surface report RPCs in team-analytics, their gateway routes, and the collector's acceptance of the `experiment_exposure` beacon.

## Requirements

### Requirement: Retention cohorts report
WHEN an admin calls `AnalyticsQueryService.GetRetentionCohorts`, THE SYSTEM SHALL return one `RetentionCohort` per non-empty weekly cohort, anchored on the UTC Monday of each user's first `sign_up` event, with `cohort_size`, `d1`, `d7`, `d30`, `weekly_active` and `repeat_purchase_30d` computed from `tracking_events_resolved` and PAID `order_facts`. `weeks` SHALL default to 12 and be at most 26.

#### Scenario: Cohort is anchored on the first sign_up week
- **WHEN** an admin calls `GetRetentionCohorts` and 3 users have a first `sign_up` on Wednesday of week W, one of them with a second `sign_up` two weeks later
- **THEN** exactly one cohort has `cohort_week` equal to the Monday of W with `cohort_size` 3
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_AnchorIsFirstSignUpWeek"

#### Scenario: Day-N retention counts the exact UTC day
- **WHEN** a cohort of 4 users has 2 users with an event on day 1 after signup, 1 with an event on day 7 and none on day 30, all three days fully elapsed
- **THEN** the cohort reports `d1` 0.5, `d7` 0.25 and `d30` 0
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_DayNIsExactDay"

#### Scenario: Weekly activity lists elapsed weeks only
- **WHEN** a cohort is 3 weeks old at the request time and 2 of its 4 users have events in week 1 after signup
- **THEN** `weekly_active` has 3 entries (weeks 0, 1 and 2) and entry 1 equals 0.5
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_WeeklyActiveOnlyElapsedWeeks"

#### Scenario: Repeat purchase uses the 30-day window
- **WHEN** a cohort has 2 buyers whose first paid orders are over 30 days old, one with a second paid order 10 days after the first and one with a second 40 days after the first
- **THEN** `repeat_purchase_30d` equals 0.5
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_RepeatPurchaseWithin30Days"

#### Scenario: A cohort too young to measure a metric reports zero
- **WHEN** a cohort signed up 3 days ago and the request asks for it
- **THEN** its `d7`, `d30` and `repeat_purchase_30d` are 0 and its `d1` is computed from the members whose day 1 has elapsed
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_YoungCohortReportsZero"

#### Scenario: Users without a sign_up event are excluded
- **WHEN** a user has events and a paid order but no `sign_up` event
- **THEN** that user is counted in no cohort and no cohort size changes
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestRetention_NoSignUpExcluded"

#### Scenario: An empty warehouse returns an empty list
- **WHEN** an admin calls `GetRetentionCohorts` on a warehouse with no `sign_up` events
- **THEN** the response has 0 cohorts and the call succeeds
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/retention_cohorts.feature` "An empty warehouse returns an empty list"

#### Scenario: Default and maximum weeks
- **WHEN** an admin calls `GetRetentionCohorts` with `weeks` 0, then with `weeks` 26
- **THEN** the first call considers 12 consecutive cohort weeks and the second 26, each starting at the UTC Monday
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestRetention_WeeksDefaultAndBoundary"

#### Scenario: Weeks above 26 is rejected
- **WHEN** an admin calls `GetRetentionCohorts` with `weeks` 27
- **THEN** the call fails with `InvalidArgument` naming `weeks` and the repository is not queried
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestRetention_WeeksAboveMaxRejected"

#### Scenario: Non-admin cannot read retention
- **WHEN** a caller with a seller principal and without scope `admin` calls `GetRetentionCohorts`
- **THEN** the call fails with `PermissionDenied` and no query runs
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestReports_NonAdminDenied"

#### Scenario: Warehouse without the capability is unavailable
- **WHEN** an admin calls `GetRetentionCohorts` and the repository does not implement the retention capability or its query fails
- **THEN** the call fails with `Unavailable` and returns no cohorts
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestReports_UnsupportedIsUnavailable"

### Requirement: Experiment readout
WHEN an admin calls `AnalyticsQueryService.GetExperimentReadout` with a `flag`, THE SYSTEM SHALL return one `ExperimentVariantRow` per variant exposed in `[from, to]`, counting each visitor once in the variant of their first exposure, `converted` as visitors with a PAID order within 7 days after that exposure, `rate = converted / exposed` and a 95% Wilson interval computed in team-analytics, and `window_days` 7. The variant `unassigned` SHALL be excluded.

#### Scenario: A visitor is counted once in the first variant
- **WHEN** one visitor has exposures to flag F as variant `a` and later as variant `b`
- **THEN** variant `a` has `exposed` 1 and variant `b` has no row
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestExperiment_FirstExposureWins"

#### Scenario: Conversion within seven days of first exposure
- **WHEN** a logged-in visitor is exposed at T and has a PAID order line at T + 6 days, and another is exposed at T and pays at T + 8 days
- **THEN** the variant has `exposed` 2 and `converted` 1 and `rate` 0.5
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestExperiment_ConversionWindowSevenDays"

#### Scenario: A paid order before the exposure does not convert
- **WHEN** a visitor has a PAID order line at T - 1 day and is first exposed at T
- **THEN** the visitor is exposed and not converted
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestExperiment_OrderBeforeExposureDoesNotConvert"

#### Scenario: Wilson interval for a known sample
- **WHEN** a variant has `exposed` 100 and `converted` 20
- **THEN** `ci_low` is 0.1334 and `ci_high` is 0.2888 to four decimals
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestWilson_KnownValues"

#### Scenario: Wilson interval boundaries
- **WHEN** a variant has `exposed` 0, or `converted` equals `exposed` at 10
- **THEN** the first has `rate`, `ci_low` and `ci_high` all 0, and the second has `ci_high` 1 and `ci_low` below 1
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestWilson_ZeroAndFullBoundaries"

#### Scenario: Unassigned and other flags are excluded
- **WHEN** exposures exist for variant `unassigned` of flag F and for a different flag G
- **THEN** the readout for F has no `unassigned` row and no row from G
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestExperiment_UnassignedAndOtherFlagsExcluded"

#### Scenario: Anonymous visitors count as exposed only
- **WHEN** an anonymous visitor with no stitched user is exposed to variant `a`
- **THEN** variant `a` counts the visitor in `exposed` and not in `converted`
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestExperiment_AnonymousNeverConverts"

#### Scenario: Unknown flag returns no variants
- **WHEN** an admin calls `GetExperimentReadout` for a flag with no exposure in the window
- **THEN** the response has 0 variants and `window_days` 7
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/experiment_readout.feature` "Unknown flag returns no variants"

#### Scenario: Missing or oversized flag is rejected
- **WHEN** an admin calls `GetExperimentReadout` with an empty `flag`, then with a 65-character `flag`
- **THEN** both calls fail with `InvalidArgument` naming `flag`
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestExperiment_FlagValidation"

#### Scenario: Inverted or oversized window is rejected
- **WHEN** an admin calls `GetExperimentReadout` with `from` after `to`, then with a window of 367 days
- **THEN** both calls fail with `InvalidArgument`
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestExperiment_WindowValidation"

#### Scenario: Window of exactly 366 days is accepted
- **WHEN** an admin calls `GetExperimentReadout` with a window of exactly 366 days
- **THEN** the call succeeds
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestExperiment_WindowBoundaryAccepted"

#### Scenario: Non-admin cannot read a readout
- **WHEN** a caller without scope `admin` calls `GetExperimentReadout`
- **THEN** the call fails with `PermissionDenied`
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestReports_NonAdminDenied"

### Requirement: AI-surface report
WHEN an admin calls `AnalyticsQueryService.GetAiSurfaceReport`, THE SYSTEM SHALL return one `AiSurfaceRow` per (surface, `model_version`) pair with `ai_interaction` events in `[from, to]`, counting `shown`, `accepted`, `edited` and `dismissed` events. An event whose action is not one of those four SHALL be ignored.

#### Scenario: Counts per surface and model version
- **WHEN** the warehouse holds for surface `assistant` and model version `mock:v1|hybrid` 5 shown, 2 accepted, 1 edited and 1 dismissed events, and 3 shown events for surface `chat_copilot` and `rules-v1`
- **THEN** the report has exactly two rows with those counts
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestAiSurface_CountsPerSurfaceAndVersion"

#### Scenario: Events outside the window are not counted
- **WHEN** an event lies 1 second before `from` and another at `to`
- **THEN** only the event at `to` is counted
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestAiSurface_WindowBoundaryInclusive"

#### Scenario: An event without model_version forms its own row
- **WHEN** an `ai_interaction` event has an empty `model_version`
- **THEN** it is reported under `model_version` equal to the empty string, not merged into another row
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestAiSurface_EmptyModelVersionOwnRow"

#### Scenario: Unknown action is ignored
- **WHEN** a stored `ai_interaction` row has action `hovered`
- **THEN** no count changes and no row appears for it
- **VERIFIED BY**: `team-analytics/internal/query/reports_test.go` "TestAiSurface_UnknownActionIgnored"

#### Scenario: Empty window returns no rows
- **WHEN** an admin calls `GetAiSurfaceReport` over a window with no `ai_interaction` events
- **THEN** the response has 0 rows and the call succeeds
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/ai_surface_report.feature` "Empty window returns no rows"

#### Scenario: Inverted window is rejected
- **WHEN** an admin calls `GetAiSurfaceReport` with `from` after `to`
- **THEN** the call fails with `InvalidArgument`
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestAiSurface_WindowValidation"

#### Scenario: Non-admin cannot read the AI report
- **WHEN** a caller without scope `admin` calls `GetAiSurfaceReport`
- **THEN** the call fails with `PermissionDenied`
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestReports_NonAdminDenied"

### Requirement: Reports return aggregates and are fast
THE SYSTEM SHALL return only counts, shares and interval bounds from the three report RPCs, never a user, anonymous or order id, and SHALL answer each with p95 at most 2 s over the widest allowed window on the local warehouse.

#### Scenario: Responses carry no identifiers
- **WHEN** the three reports run over a fixture whose user ids, anonymous ids and order ids are known strings
- **THEN** none of those strings appears in the serialised responses, in error text or in log output
- **VERIFIED BY**: `team-analytics/internal/rpc/reports_test.go` "TestReports_NoIdentifiersLeak"

#### Scenario: Report latency within budget
- **WHEN** 30 admin calls of each report run over 26 weeks (or 366 days) on a warehouse with 200,000 tracking events and 20,000 paid order lines
- **THEN** the p95 of each report is at most 2 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/experiment_reports_access.feature` "Report latency within budget"

### Requirement: Gateway serves the reports to admins only
WHEN a request for `GetRetentionCohorts`, `GetExperimentReadout` or `GetAiSurfaceReport` reaches team-gateway, THE SYSTEM SHALL forward it to team-analytics only for a verified principal holding scope `admin`, answering an anonymous caller 401 and a caller without the scope 403.

#### Scenario: Admin reads each report through the gateway
- **WHEN** the seeded admin calls the three procedures through the gateway
- **THEN** each returns 200 with the team-analytics response unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/experiment_reports_access.feature` "Admin reads each report through the gateway"

#### Scenario: Buyer or seller is refused at the edge
- **WHEN** a logged-in buyer, then a logged-in seller, calls each of the three procedures through the gateway
- **THEN** every call returns 403 and team-analytics receives no request
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/experiment_reports_access.feature` "Buyer or seller is refused at the edge"

#### Scenario: Anonymous caller is refused at the edge
- **WHEN** a caller without a token calls each of the three procedures through the gateway
- **THEN** every call returns 401
- **VERIFIED BY**: `team-gateway/internal/edge/policy_test.go` "TestPolicy_ReportProceduresRequireAuthentication"

#### Scenario: Procedures are registered as admin-gated
- **WHEN** `adminProcedures` is inspected
- **THEN** the three report procedures map to `adminScope`
- **VERIFIED BY**: `team-gateway/internal/edge/policy_test.go` "TestPolicy_ReportProceduresAreAdminGated"

#### Scenario: Upstream failure is surfaced, not masked
- **WHEN** team-analytics answers `Unavailable` to a forwarded report call
- **THEN** the gateway returns the same error code to the admin and no partial body
- **VERIFIED BY**: `team-gateway/internal/edge/analytics_test.go` "TestExperimentReportForwarder_UnavailablePassesThrough"

### Requirement: Collector accepts the experiment exposure beacon
WHEN the beacon collector receives an `experiment_exposure` event, THE SYSTEM SHALL accept it only with properties of exactly `flag` and `variant`, each matching `^[a-z0-9][a-z0-9_.-]{0,63}$`, and publish it as `EVENT_TYPE_EXPERIMENT_EXPOSURE`; team-analytics SHALL store it with `event_type` `experiment_exposure` and the two properties.

#### Scenario: A valid exposure is stored
- **WHEN** a visitor posts a beacon of type `experiment_exposure` with properties `flag` `home-feed-ranker` and `variant` `treatment`
- **THEN** the warehouse holds one row with `event_type` `experiment_exposure` and `properties` equal to those two keys and values
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/tracking/experiment_exposure.feature` "A valid exposure is stored"

#### Scenario: A third property is rejected
- **WHEN** a beacon of type `experiment_exposure` carries `flag`, `variant` and `note`
- **THEN** the collector drops it with the invalid-properties error and nothing is published
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` "TestCollector_ExperimentExposureRejectsExtraProperty"

#### Scenario: Free text or an email in a value is rejected
- **WHEN** a beacon of type `experiment_exposure` has `variant` `Jane Doe jane@example.com`
- **THEN** the collector drops it and nothing is published
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` "TestCollector_ExperimentExposureRejectsFreeText"

#### Scenario: Missing property is rejected
- **WHEN** a beacon of type `experiment_exposure` has only `flag`
- **THEN** the collector drops it and nothing is published
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` "TestCollector_ExperimentExposureRejectsMissingVariant"

#### Scenario: Value length boundary
- **WHEN** beacons carry a 64-character `variant`, then a 65-character `variant`
- **THEN** the first is published and the second is dropped
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` "TestCollector_ExperimentExposureValueLengthBoundary"

#### Scenario: Existing event types are unaffected
- **WHEN** the previously accepted beacon types, including `ai_interaction` with its closed properties, are posted after the change
- **THEN** each is accepted or rejected exactly as before
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` "TestCollector_ExperimentExposureExistingTypesUnchanged"

#### Scenario: Consumer names the new event type
- **WHEN** team-analytics maps a `TrackingEvent` of type `EVENT_TYPE_EXPERIMENT_EXPOSURE`
- **THEN** the stored record has `EventType` `experiment_exposure` and keeps the `flag` and `variant` properties
- **VERIFIED BY**: `team-analytics/internal/events/tracking_test.go` "TestTracking_ExperimentExposureEventName"
