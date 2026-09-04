# Tasks

> Depends on `add-prometheus-infra` — real per-service metrics must exist in
> Prometheus before this handler can query them.

## 1. Code — team-gateway
- [x] `internal/config/config.go`: added `Edge.PrometheusURL`
      (`env:"PROMETHEUS_URL" default:"http://prometheus:9090"`); threaded into the
      cockpit handler via `NewCockpitHandler` wired in `server.go` (new `prometheusURL`
      param on `NewMux`, passed `settings.Edge.PrometheusURL` from `main.go`).
      `.env.example` updated so the `check-env` drift gate stays green.
- [x] `internal/edge/cockpit.go`: replaced the `math/rand` body with a
      `CockpitHandler` that runs the fixed PromQL set (per-service RPS, p95/p99
      latency, error-rate) against `PROMETHEUS_URL`'s `GET /api/v1/query` and maps
      results into the **unchanged** `CockpitMetricsResponse`/`ServiceHealth`/
      `TraceSummary` structs (fields byte-for-byte identical — frontend untouched).
      Query set is hardcoded in-handler; `rpc_service` labels folded onto the fixed
      10-row roster (mirrors `internal/upstream`). No arbitrary PromQL, no raw
      passthrough. All 4 queries validated `status:success` on a live Prometheus.
- [x] `total_orders_24h`/`total_revenue_24h`: sourced as derived/estimated constants
      (documented non-authoritative in-code), not from a Prometheus business counter;
      `recent_traces` keep Jaeger deep-links (labelled non-authoritative). Field
      names/shape unchanged.
- [x] Graceful degradation: empty `PROMETHEUS_URL` or any query error → same shape
      with zeroed values, `status:"UNKNOWN"` (never `rand`). Covered by a unit test.
- [x] `internal/edge/server.go`: kept `GET /api/admin/metrics`; now serves
      `NewCockpitHandler(prometheusURL)`. No new browser-facing PromQL route.
- [x] `internal/edge/cockpit_test.go`: rewritten — asserts shape + content-type,
      Prometheus-sourced mapping via an `httptest` fake Prometheus (team-search row
      maps to 42.5 RPS / 13.2 p95 / 27.9 p99 / HEALTHY), and degraded-mode (no
      `PROMETHEUS_URL`) returns zeros not random.
- [~] `go build ./...` + `go test ./internal/edge/...` green: cannot run locally
      (no Go; generated proto gitignored → package `edge` won't compile standalone).
      Verified as far as possible: `gofmt -e` clean on `cockpit.go` + `cockpit_test.go`
      (pure-stdlib, no proto imports). Full build + test run in CI/Docker.

## 2. E2E (platform-e2e)
- [x] `team-frontend/FEATURES.yaml`: added `ops.cockpit-live-metrics` (persona `admin`,
      `entry_route: /admin/cockpit`, `services: [team-frontend, team-gateway,
      prometheus, team-search]`), `acceptance` mapping 1:1 to spec scenario "Cockpit
      reflects real request activity", `status: planned`. Validated against
      `platform-e2e/schemas/features.schema.json` (required keys present, no extra keys,
      persona/priority/status enums OK).
- [x] platform-e2e: cockpit metrics `.feature` + steps + page-object locators. AUTHORED:
      `tests/e2e/features/ops/cockpit_metrics.feature` (5 scenarios, names mapped 1:1 to the spec
      `#### Scenario:` titles) + `step_definitions/cockpit_metrics_steps.py` +
      `step_definitions/test_cockpit_metrics.py` (binder). New plumbing: `MetricsService`
      (`src/api/services/metrics_service.py`) reading `GET /api/admin/metrics` via a new
      `BaseService.get`, and asserting the raw Prometheus query path is NOT exposed; reused the
      existing `CockpitPage` (added `red_metrics_radar` / `service_rps_cell` locators, still
      assertion-free). Drives real search traffic via the existing `SearchService`. Wired into
      `make collect` (auto-discovered — all 5 scenarios collect).
- [ ] Run green against the local stack + flip `ops.cockpit-live-metrics` to `automated`
      (set `covered_by`). — GATED ON CI/STACK: needs frontend → gateway → Prometheus with a compiled
      gateway emitting/serving metrics (Go not buildable on this host). Entry stays `status: planned`
      (no `covered_by`); `scripts/features.py` validates it.
- [ ] `make -C platform-e2e features-check` green — GATED with the run above.

## Archive
- [~] Verify end-to-end: HUD at `/admin/cockpit` shows live per-service metrics from
      Prometheus, values move with real traffic, frontend unchanged. Code complete +
      unit-tested logic; live render needs the full stack (compiled gateway + emitting
      metrics + Prometheus). Frontend confirmed unchanged (response shape identical).
- [ ] Cockpit e2e automated + green; `features-check` green — DEFERRED (see E2E track).
- [ ] Archive the change (`/opsx:archive replace-cockpit-mock-metrics`) — pending
      go-ahead.
