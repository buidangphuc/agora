## Why

The docs-ai-data initiative adds three admin reports (retention cohorts, experiment readout, AI-surface report), a flag-exposure tracking event, provenance fields on the AI responses (`request_id`, `model_version`) and a forecast run time. Every consumer slice (team-analytics, team-gateway, team-ai, the three frontends) needs these on the wire first. All of it is additive proto owned by platform-core. Shipping it first, with vendored consumer copies refreshed, lets the other slices build.

initiative: docs-ai-data

## What Changes

All edits are in `platform-core/packages/proto/platform`; no field is renamed, renumbered or removed.

- `analytics/v1/analytics.proto`:
  - `EventType` gains `EVENT_TYPE_EXPERIMENT_EXPOSURE = 22`, commented: a server-evaluated feature-flag variant was shown, properties exactly `flag` and `variant`, no free text.
  - `GetDemandForecastResponse` gains `google.protobuf.Timestamp generated_at = 8`, the forecast run time, unset when `is_cold_start`.
  - `AnalyticsQueryService` gains `GetRetentionCohorts`, `GetExperimentReadout` and `GetAiSurfaceReport`, admin-only (scope `admin`), with the request and response messages `GetRetentionCohortsRequest`, `RetentionCohort`, `GetRetentionCohortsResponse`, `GetExperimentReadoutRequest`, `ExperimentVariantRow`, `GetExperimentReadoutResponse`, `GetAiSurfaceReportRequest`, `AiSurfaceRow`, `GetAiSurfaceReportResponse` at the field numbers of the initiative spec.
- `ai/v1/ai.proto`: `ShoppingAssistantResponse.request_id = 4` and `model_version = 5`, `MagicListingResponse.request_id = 7` and `model_version = 8`, `ChatCopilotResponse.request_id = 2` and `model_version = 3`, `SummarizeReviewsResponse.request_id = 5` and `model_version = 6`. Comments: `request_id` is a UUID hex minted per call, `model_version` is `<backend>:<prompt_version>` for LLM surfaces and `rules-v1` for rule surfaces, and the assistant appends `|<search model_version>`.
- Proto comments document the semantics: the weeks bound (1..26, 0 means 12), the cohort week as a UTC Monday, shares in 0..1, the 95% Wilson interval, the conversion window of 7 days after the first exposure, aggregates only (no ids), and that `ci_low`/`ci_high` are computed server-side.
- Baseline for every compatibility check: the `AnalyticsQueryService` rpc table, the `EventType` table, `GetDemandForecastResponse` and the four AI response messages, pinned in the test files, copied from `origin/main` at the time of this change.
- Vendored copies under `<repo>/proto/platform/...` are refreshed byte-for-byte in every consumer repo that already holds the file: analytics.proto (team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway) and ai.proto (team-ai, team-analytics, team-domain, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-notification, team-order, team-search). Stubs checked in by a consumer are regenerated with that repo's own `buf generate`.

## Capabilities

### New Capabilities
- `docs-ai-data-contract`: the proto contract for the three report RPCs, `EVENT_TYPE_EXPERIMENT_EXPOSURE`, forecast `generated_at` and the AI provenance fields.

### Modified Capabilities
none

## Impact

- `platform-core/packages/proto/platform/{analytics,ai}/v1/*.proto`, new `aidata_compat_test.go` files beside them and `aidata_consumer_copies_test.sh`.
- Vendored `proto/` copies and regenerated stubs in the consumer repos listed above (copy refresh only, no behaviour change).
- team-gateway gets the refreshed copy but no route here: the routes ship in `analytics-experiment-reports`.

## Non-goals

Implementing or routing the RPCs (team-analytics and team-gateway, slice `analytics-experiment-reports`), writing the AI provenance values (team-ai slices), emitting or allowlisting the exposure event (team-frontend and `analytics-experiment-reports`), serving `generated_at` (slice `analytics-forecast-serving`). No change to `GetRecommendationPerformance`, `GetSearchPerformance`, `RecommendResponse` or `SearchHit`. No new Flipt or Redis contract.

## Non-functional requirements

- `buf lint` and `buf breaking` (FILE) against `origin/main` pass: purely additive.
- Wire compatibility: a payload from the previous schema parses with the new one, and the reverse, with existing fields unchanged. A consumer that meets event type 22 before it is updated reads an unknown enum number and must not crash.
- Availability: no runtime component; rollout is proto first, consumers after.
- Latency budgets for the report RPCs (p95 at most 2 s over 26 weeks) and for `GetDemandForecast` (p95 at most 30 ms) are met and tested in the consumer slices.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `RetentionCohort.*`, `ExperimentVariantRow.*`, `AiSurfaceRow.*` | no: aggregate counts and shares, no ids | admin reporting | not applicable to the contract; basis for counting first-party events is decision U6 (DPO sign-off, human item) | not stored by the contract |
| `GetExperimentReadoutRequest.flag` | no: Flipt flag key | select the experiment | not applicable | not stored |
| `EVENT_TYPE_EXPERIMENT_EXPOSURE` properties `flag`, `variant` | no: allowlisted keys, no free text; visitor identity travels only in `anonymous_id`/`principal` as for every event | record that a variant was shown | as for other first-party events (U6) | as the `analytics.events` pipeline |
| `request_id`, `model_version` on AI responses | no: random UUID and a model label; no buyer id | tie an AI event to the response that produced it | not applicable | carried on events, retention as the warehouse |
| `GetDemandForecastResponse.generated_at` | no: job run time | show forecast freshness | not applicable | not stored by the contract |

## Threat model

- Assets: aggregate business reports (retention, experiment results) that are commercially sensitive but hold no personal data.
- Abuse cases: a non-admin reading reports; a request with a huge `weeks` or an inverted window used as load amplification; an unbounded `flag` string; free text smuggled into exposure events.
- Authorisation rule: all three report RPCs are admin-only. The contract documents scope `admin`; enforcement lives in team-gateway (403) and team-analytics (`PermissionDenied`) in `analytics-experiment-reports`. The `weeks` bound (1..26) is part of the contract. The exposure event keeps the `properties` allowlist (exactly `flag`, `variant`) documented in the enum comment and enforced by the collector.
