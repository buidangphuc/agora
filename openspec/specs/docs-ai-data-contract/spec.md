# docs-ai-data-contract Specification

## Purpose
Defines the additive platform-core proto surface of the docs-ai-data initiative: the three admin report RPCs (retention cohorts, experiment readout, AI-surface report), the flag-exposure tracking event, provenance fields on AI responses and the forecast run time.

## Requirements

### Requirement: Experiment exposure event type
The analytics proto SHALL add `EVENT_TYPE_EXPERIMENT_EXPOSURE = 22` to `EventType`, commented as a server-evaluated feature-flag variant that was shown, with properties exactly `flag` and `variant` and no free text. Values 0 to 21 SHALL keep their names and numbers.

#### Scenario: Exposure value present at 22
- **WHEN** the descriptor of `platform.analytics.v1.EventType` is inspected
- **THEN** it has `EVENT_TYPE_EXPERIMENT_EXPOSURE=22` and exactly 23 values in total
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ExposureEventTypeIsTwentyTwo"

#### Scenario: Existing event types keep their numbers
- **WHEN** the `EventType` descriptor is compared with the baseline pinned in the test
- **THEN** every value 0 to 21 is identical in name and number, including `EVENT_TYPE_AI_INTERACTION=21`
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ExistingEventTypesUnchanged"

#### Scenario: Exposure properties are documented as allowlisted
- **WHEN** the comment on `EVENT_TYPE_EXPERIMENT_EXPOSURE` is read
- **THEN** it states properties exactly `flag` and `variant` and no free text
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ExposureDocumented"

#### Scenario: Old consumer meets the new event type
- **WHEN** a `TrackingEvent` with type 22 is parsed by the previous schema
- **THEN** parsing succeeds, the type reads as number 22 with no defined name, and every other field is unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_OldSchemaParsesExposure"

#### Scenario: Undefined event number is not a defined value
- **WHEN** the `EventType` descriptor is searched for number 23
- **THEN** no value exists, so the check fails if a value is added without a spec change
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_NoUndefinedEventNumbers"

### Requirement: Forecast run time
`GetDemandForecastResponse` SHALL add `google.protobuf.Timestamp generated_at = 8`, documented as the forecast run time and unset when `is_cold_start` is true. Fields 1 to 7 SHALL be unchanged.

#### Scenario: generated_at present at 8
- **WHEN** the descriptor of `GetDemandForecastResponse` is inspected
- **THEN** it has `generated_at=8` of type `google.protobuf.Timestamp` and fields 1 to 7 are identical to the baseline pinned in the test
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ForecastGeneratedAtShape"

#### Scenario: Unset run time on a cold start is distinct from a set one
- **WHEN** a response with `is_cold_start=true` and no `generated_at`, and one with `generated_at` set, are serialised and parsed
- **THEN** the first reads presence false for `generated_at`, the second reads presence true with the same instant, and the bytes differ
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_GeneratedAtPresenceRoundTrip"

#### Scenario: Cold-start semantics are documented
- **WHEN** the comment on `generated_at` is read
- **THEN** it states the forecast run time and that it is unset when `is_cold_start` is true
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_GeneratedAtDocumented"

#### Scenario: Forecast fields 1 to 7 are not renumbered
- **WHEN** the `GetDemandForecastResponse` descriptor is compared with the baseline and a field is missing or renumbered
- **THEN** the check fails, so no existing forecast field can be removed without a spec change
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ForecastExistingFieldsUnchanged"

### Requirement: Admin report RPCs
`AnalyticsQueryService` SHALL add `rpc GetRetentionCohorts(GetRetentionCohortsRequest) returns (GetRetentionCohortsResponse)`, `rpc GetExperimentReadout(GetExperimentReadoutRequest) returns (GetExperimentReadoutResponse)` and `rpc GetAiSurfaceReport(GetAiSurfaceReportRequest) returns (GetAiSurfaceReportResponse)`. Each rpc comment SHALL state admin-only (scope `admin`) and that the output is aggregate counts with no ids. The messages SHALL have the fields and numbers listed in the initiative spec: `GetRetentionCohortsRequest {from=1 Timestamp, weeks=2 uint32}`, `RetentionCohort {cohort_week=1 string, cohort_size=2 int64, d1=3, d7=4, d30=5 double, weekly_active=6 repeated double, repeat_purchase_30d=7 double}`, `GetRetentionCohortsResponse {cohorts=1 repeated}`, `GetExperimentReadoutRequest {flag=1 string, from=2, to=3 Timestamp}`, `ExperimentVariantRow {variant=1 string, exposed=2 int64, converted=3 int64, rate=4, ci_low=5, ci_high=6 double}`, `GetExperimentReadoutResponse {variants=1 repeated, window_days=2 uint32}`, `GetAiSurfaceReportRequest {from=1, to=2 Timestamp}`, `AiSurfaceRow {surface=1, model_version=2 string, shown=3, accepted=4, edited=5, dismissed=6 int64}`, `GetAiSurfaceReportResponse {rows=1 repeated}`.

#### Scenario: Three RPCs present with their message types
- **WHEN** the descriptor of `AnalyticsQueryService` is inspected
- **THEN** it has `GetRetentionCohorts`, `GetExperimentReadout` and `GetAiSurfaceReport`, each from its request type to its response type, and no streaming
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ReportRpcsShape"

#### Scenario: Message fields sit at the specified numbers
- **WHEN** the descriptors of the nine new messages are inspected
- **THEN** every field has the name, number, type and repeated flag listed in this requirement
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ReportMessagesShape"

#### Scenario: Existing query surface is untouched
- **WHEN** the earlier `AnalyticsQueryService` rpcs and messages are compared with the baseline pinned in the test
- **THEN** every earlier rpc, message, field name, number and type is identical, including `GetRecommendationPerformance` and `GetSearchPerformance`
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ExistingQuerySurfaceUntouched"

#### Scenario: Admin-only and aggregate-only are documented
- **WHEN** the comments on the three rpcs are read
- **THEN** each contains `scope "admin"` and states that no ids or per-user data are returned
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ReportRpcsDocumentedAdminOnly"

#### Scenario: Window and interval semantics are documented
- **WHEN** the comments on `GetRetentionCohortsRequest.weeks`, `RetentionCohort`, `ExperimentVariantRow` and `GetExperimentReadoutResponse.window_days` are read
- **THEN** they state weeks `1..26` with 0 meaning 12 and out of range as INVALID_ARGUMENT, the cohort week as a UTC Monday, shares in 0..1, the 95% Wilson interval, and the conversion window as a paid order within 7 days of the first exposure
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ReportSemanticsDocumented"

#### Scenario: Empty report is a valid response
- **WHEN** a response with zero cohorts, zero variants or zero rows is serialised and parsed
- **THEN** each parses to an empty list and no error, so the schema never forces a fabricated row
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_EmptyReportsRoundTrip"

#### Scenario: Wire compatible both ways
- **WHEN** bytes of an `AnalyticsQueryService` message serialised by the previous schema are parsed with the new one, and a `GetDemandForecastResponse` with `generated_at` is parsed by the previous schema
- **THEN** both parses succeed and fields 1 to 7 are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_WireCompatible"

#### Scenario: The report messages carry no personal data
- **WHEN** the field names of the nine new messages are scanned
- **THEN** none is named `user_id`, `anonymous_id`, `session_id`, `email`, `phone` or `ip`
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/aidata_compat_test.go` "TestAiData_ReportsCarryNoPersonalData"

### Requirement: AI response provenance fields
The AI proto SHALL add `string request_id` and `string model_version` to `ShoppingAssistantResponse` (4, 5), `MagicListingResponse` (7, 8), `ChatCopilotResponse` (2, 3) and `SummarizeReviewsResponse` (5, 6). The comments SHALL state that `request_id` is a UUID hex minted per call, that `model_version` is `<backend>:<prompt_version>` for LLM surfaces and `rules-v1` for rule surfaces, and that the assistant appends `|<search model_version>`. Existing fields SHALL be unchanged.

#### Scenario: Provenance fields present at the specified numbers
- **WHEN** the descriptors of the four response messages are inspected
- **THEN** each has `request_id` and `model_version` as strings at the numbers listed in this requirement
- **VERIFIED BY**: `platform-core/packages/proto/platform/ai/v1/aidata_compat_test.go` "TestAiData_ProvenanceFieldsShape"

#### Scenario: Existing AI surface is untouched
- **WHEN** the `AIService` rpcs and the message fields that existed before this change are compared with the baseline pinned in the test
- **THEN** every earlier rpc, message, field name, number and type is identical
- **VERIFIED BY**: `platform-core/packages/proto/platform/ai/v1/aidata_compat_test.go` "TestAiData_ExistingAiSurfaceUntouched"

#### Scenario: Provenance format is documented
- **WHEN** the comments on the new fields are read
- **THEN** they state UUID hex per call, `<backend>:<prompt_version>`, `rules-v1` and the assistant's `|<search model_version>` suffix
- **VERIFIED BY**: `platform-core/packages/proto/platform/ai/v1/aidata_compat_test.go` "TestAiData_ProvenanceDocumented"

#### Scenario: Old client reads a new response
- **WHEN** a new `ShoppingAssistantResponse` with both fields set is parsed by the previous schema, and an old response is parsed with the new one
- **THEN** both parse, the old reader keeps `reply_text`, `product_cards` and `suggested_followups`, and the new reader sees empty `request_id` and `model_version`
- **VERIFIED BY**: `platform-core/packages/proto/platform/ai/v1/aidata_compat_test.go` "TestAiData_WireCompatible"

#### Scenario: Unset provenance is an empty string, never a placeholder
- **WHEN** a response is built without `request_id` and `model_version` and serialised
- **THEN** both read as the empty string and add no bytes to the payload
- **VERIFIED BY**: `platform-core/packages/proto/platform/ai/v1/aidata_compat_test.go` "TestAiData_UnsetProvenanceIsEmpty"

### Requirement: Vendored consumer copies match the source
Every consumer repo that holds `analytics.proto` or `ai.proto` SHALL carry a byte-identical copy of the platform-core file, and `buf lint` and `buf breaking` (FILE) against `origin/main` SHALL pass.

#### Scenario: Copies match the source
- **WHEN** each vendored `proto/platform/analytics/v1/analytics.proto` (5 repos) and `proto/platform/ai/v1/ai.proto` (10 repos) is compared with the platform-core file
- **THEN** all 15 copies are byte-identical
- **VERIFIED BY**: `platform-core/packages/proto/aidata_consumer_copies_test.sh` "copies-match"

#### Scenario: Drifted or missing copy is rejected
- **WHEN** one vendored copy differs by a byte or is absent
- **THEN** the script exits non-zero and names the repo and file
- **VERIFIED BY**: `platform-core/packages/proto/aidata_consumer_copies_test.sh` "drift-rejected"

#### Scenario: Renumbering is rejected
- **WHEN** an existing field or enum value is renumbered or removed in the source proto
- **THEN** `buf breaking --against '.git#branch=origin/main'` fails and the script exits non-zero
- **VERIFIED BY**: `platform-core/packages/proto/aidata_consumer_copies_test.sh` "breaking-rejected"

#### Scenario: Consumer protos still build and lint
- **WHEN** `buf lint` runs in `platform-core/packages/proto` and `buf build` runs on each consumer's vendored tree
- **THEN** all exit 0 with no findings
- **VERIFIED BY**: `platform-core/packages/proto/aidata_consumer_copies_test.sh` "lint-and-build"
