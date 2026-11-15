## 1. Code track: team-analytics

Builds after `analytics-forecast-serving` (shared `handler.go`, `repository.go`, `duckdb.go`) and after `docs-ai-data-contract` re-vendored `team-analytics/proto`.

- [x] 1.1 Tests first, verify red: `internal/query/reports_test.go` over a DuckDB fixture (Cohort is anchored on the first sign_up week; Day-N retention counts the exact UTC day; Weekly activity lists elapsed weeks only; Repeat purchase uses the 30-day window; A cohort too young to measure a metric reports zero; Users without a sign_up event are excluded; A visitor is counted once in the first variant; Conversion within seven days of first exposure; A paid order before the exposure does not convert; Unassigned and other flags are excluded; Anonymous visitors count as exposed only; Counts per surface and model version; Events outside the window are not counted; An event without model_version forms its own row; Unknown action is ignored)
- [x] 1.2 Tests first, verify red: `internal/rpc/reports_test.go` with the spy/memory repository (Default and maximum weeks; Weeks above 26 is rejected; Non-admin cannot read retention; Warehouse without the capability is unavailable; Wilson interval for a known sample; Wilson interval boundaries; Missing or oversized flag is rejected; Inverted or oversized window is rejected; Window of exactly 366 days is accepted; Non-admin cannot read a readout; Inverted window is rejected; Non-admin cannot read the AI report; Responses carry no identifiers)
- [x] 1.3 Tests first, verify red: `internal/events/tracking_test.go` `TestTracking_ExperimentExposureEventName` (Consumer names the new event type)
- [x] 1.4 `internal/query/repository.go`: add `RetentionRepository`, `ExperimentRepository`, `AiSurfaceRepository` and their row structs (counts only); implement them on `DuckDBRepository` in new `internal/query/reports.go` per design D3, D4, D6; no new table or column (all query scenarios above)
- [x] 1.5 `internal/rpc/reports.go`: three handlers mirroring `GetChannelFunnel` (`requireAdmin` first, validation per design D2, capability check, `s.fail`); pure `wilson(converted, exposed)` per D5; no value from data in errors or logs (all handler scenarios; Responses carry no identifiers)
- [x] 1.6 `internal/events/tracking.go`: map `EVENT_TYPE_EXPERIMENT_EXPOSURE` to `experiment_exposure` (Consumer names the new event type)
- [x] 1.7 README: report semantics (anchor, day-N, 7-day window, anonymous undercount, DuckDB only); update the config/README doc tests if they pin the RPC list
- [x] 1.8 `go test ./...` with `GO_SDK_REQUIRE_DOCKER=1`; repo gate via `gate_digest.py`; no coverage drop

## 2. Code track: team-gateway

- [x] 2.1 Tests first, verify red: `internal/edge/policy_test.go` (Anonymous caller is refused at the edge; Procedures are registered as admin-gated), `internal/edge/analytics_test.go` (Upstream failure is surfaced, not masked), `internal/edge/collector_test.go` (A third property is rejected; Free text or an email in a value is rejected; Missing property is rejected; Value length boundary; Existing event types are unaffected)
- [x] 2.2 `internal/edge/policy.go`: add the three `analyticsv1connect.AnalyticsQueryService*Procedure` constants to `adminProcedures` with `adminScope` (Procedures are registered as admin-gated, Buyer or seller is refused at the edge)
- [x] 2.3 `internal/edge/analytics.go`: three forwarder methods in the style of `GetChannelFunnel` (Admin reads each report through the gateway, Upstream failure is surfaced, not masked)
- [x] 2.4 `internal/edge/collector.go`: add `"experiment_exposure"` to `beaconEventTypes`; in `validateBeacon` require exactly `flag` and `variant` matching `^[a-z0-9][a-z0-9_.-]{0,63}$` (A third property is rejected, Free text or an email in a value is rejected, Missing property is rejected, Value length boundary)
- [x] 2.5 `go test ./...` and the repo gate; no coverage drop

## 3. E2E track (platform-e2e)

- [x] 3.1 `tests/e2e/features/analytics/retention_cohorts.feature`: "An empty window returns no cohorts" plus a seeded scenario with users signed up through the gateway (sign_up beacons with login principal) whose cohort row is read by the admin; scenario titles equal the spec titles (Cohort is anchored on the first sign_up week, An empty warehouse returns an empty list)
- [x] 3.2 `tests/e2e/features/analytics/experiment_readout.feature`: exposure beacons under a fresh flag key per scenario, one buyer paying an order, admin reads variants (Conversion within seven days of first exposure, A visitor is counted once in the first variant, Unknown flag returns no variants)
- [x] 3.3 `tests/e2e/features/analytics/ai_surface_report.feature`: `ai_interaction` beacons under a fresh `model_version`, admin reads the row (Counts per surface and model version, An empty window returns no rows)
- [x] 3.4 `tests/e2e/features/analytics/experiment_reports_access.feature`: admin 200, buyer and seller 403 for each RPC, p95 latency over the seeded warehouse (Admin reads each report through the gateway, Non-admins get 403 on every report, Report latency within budget)
- [x] 3.5 `tests/e2e/features/tracking/experiment_exposure.feature`: valid beacon stored with type `experiment_exposure`; invalid properties drop (A valid exposure is stored)
- [x] 3.6 FEATURES.yaml entries for team-analytics and team-gateway via the e2e-author (not written by this change); step definitions reuse the search-performance admin steps and the beacon poster
- [x] 3.7 Local run with only redpanda, team-analytics, team-gateway, team-identity and the services the seeded buyer needs; stop them after
- [x] 3.8 `openspec validate analytics-experiment-reports --strict`, `spec_lint`, per-repo checks, pre-commit-ci
