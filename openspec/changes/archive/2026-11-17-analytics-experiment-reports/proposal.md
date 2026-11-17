## Why

Admins cannot tell whether a recommendation arm, an AI surface or a new cohort performs: `GetRecommendationPerformance` and `GetSearchPerformance` exist but no report covers retention, experiment outcomes or AI-surface usage, and the gateway collector rejects the flag-exposure beacon that experiments need. This slice implements the three admin report RPCs fixed by `docs-ai-data-contract`, routes them through the gateway as admin-only, and lets the collector accept `experiment_exposure`.

initiative: docs-ai-data

## What Changes

- team-analytics: `AnalyticsQueryService.GetRetentionCohorts`, `GetExperimentReadout` and `GetAiSurfaceReport` in `internal/rpc` (each `requireAdmin`, mirroring `GetChannelFunnel`), backed by new repository capability interfaces in `internal/query` implemented by `DuckDBRepository` over `tracking_events_resolved` and `order_facts`. A repository without the capability returns `query.ErrUnsupported`, which the handler maps to `Unavailable`. Wilson intervals are computed in the handler, not in SQL.
- team-analytics: the consumer's event-type name table (`internal/events/tracking.go`) maps `EVENT_TYPE_EXPERIMENT_EXPOSURE` to `experiment_exposure`, so the row is stored with a readable `event_type` and `properties` `{flag, variant}`.
- team-gateway: the three procedures join `adminProcedures` with `adminScope`; `AnalyticsQueryForwarder` forwards them; the beacon collector gains `experiment_exposure` with a closed property check (exactly `flag` and `variant`, each a short lowercase key).
- No warehouse schema change and no new table: the reports read existing columns (`properties`, `model_version`, `user_key`, `buyer_id`, `status`).
- Vendored proto copies in team-analytics and team-gateway are already refreshed by `docs-ai-data-contract`.
- FEATURES.yaml entries are written by the e2e-author, not by this change.

## Capabilities

### New Capabilities
- `analytics-experiment-reports`: admin-only retention, experiment-readout and AI-surface reports, their gateway routes, and the exposure beacon allowlist.

### Modified Capabilities
none

## Impact

- team-analytics: `internal/query/{repository.go,reports.go}`, `internal/rpc/{handler.go,reports.go}`, `internal/events/tracking.go`, README. `repository.go`, `duckdb.go` and `handler.go` are shared with `analytics-forecast-serving`; this slice builds after it and keeps its new code in new files (`reports.go`) to limit conflicts.
- team-gateway: `internal/edge/{policy.go,analytics.go,collector.go}`.
- platform-e2e: new analytics features.
- Consumers: `admin-insights-panels` (cockpit) and `buyer-ai-surfaces-ui` (emits the exposure beacon).

## Non-goals

- UI for the reports (`admin-insights-panels`), emitting beacons (`buyer-ai-surfaces-ui`), flag evaluation and arm stamping (`recs-experiment-arms`).
- A BigQuery implementation of the reports: only the DuckDB warehouse has query repositories today; BigQuery returns `Unavailable`.
- Auto-promotion, significance tests beyond the 95% Wilson interval, multiple-flag joins, per-user drill-down.
- Changes to `GetRecommendationPerformance` and `GetSearchPerformance`.

## Non-functional requirements

- Latency: each report RPC p95 at most 2 s over the widest allowed window (26 weeks, or 366 days for the range reports) on the local warehouse seeded with 200,000 tracking events and 20,000 paid order lines.
- Availability: read-only, no new dependency; a warehouse failure returns `Unavailable` and no partial numbers.
- Limits: `weeks` 1..26 (0 means 12); `[from, to]` at most 366 days; `flag` at most 64 characters; response size bounded by 26 cohorts, by the variants of one flag, and by the surface x model_version pairs in the window.
- Correctness: a report never invents a number; an empty window returns an empty list; a cohort too young to measure a metric reports 0 for it (see design D3).
- Compatibility: additive; the collector keeps accepting every existing event type unchanged.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| Report responses (`RetentionCohort`, `ExperimentVariantRow`, `AiSurfaceRow`) | no: counts and shares, never ids | admin reporting on retention, experiments, AI surfaces | first-party events counted without filtering on `consent_analytics`, as the existing funnels do (decision U6; DPO sign-off is a human item) | not stored; computed per call |
| `experiment_exposure` beacon properties `flag`, `variant` | no: flag key and variant name only, no free text | exposure denominator for readouts | same as U6 | existing `tracking_events` retention, no new store |
| Warehouse `user_key`, `buyer_id`, `principal_id`, `anonymous_id` read inside SQL | yes: pseudonymous ids | joining exposure, activity and paid orders per visitor | same as U6 | existing tables; ids never leave the query, never logged, never returned |

## Threat model

- Assets: behavioural events and paid-order facts of every user; the pseudonymous id join.
- Abuse cases: (1) a seller or buyer reads platform-wide retention or revenue-adjacent rates; (2) a visitor forges exposure beacons to skew a readout or smuggle free text or personal data through `flag`/`variant`; (3) an expensive window or huge `weeks` exhausts the warehouse; (4) ids or emails leak through error text or logs; (5) a tiny variant exposes an individual's behaviour.
- Authz rule: all three RPCs require scope `admin`; the gateway returns 401 anonymous and 403 without the scope, team-analytics repeats the check (`PermissionDenied`) as defence in depth. Responses hold aggregates only. The collector accepts an exposure beacon only with exactly two keys, each matching `^[a-z0-9][a-z0-9_.-]{0,63}$`, so no free text or id-shaped value is stored. Bounds on `weeks`, window length and `flag` cap cost. Errors name the field, never a value from the data. Small variants are reported as counts; the admin audience and aggregate shape are the mitigation, no suppression threshold is added (see OPEN_ISSUES in the hand-off).
