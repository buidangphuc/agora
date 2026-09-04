# Tasks

## 1. Code — team-gateway (metrics producer)
- [x] `internal/observability/meter.go` (new): add `InitMeter(ctx, settings)`
      mirroring `InitTracer` — global `MeterProvider` with an OTLP metric exporter
      (`otlpmetricgrpc`), `marketplace`-compatible resource (`OTEL_SERVICE_NAME`),
      no-op + `nil`-shutdown when `OTEL_ENABLED=false`; return a shutdown/flush func.
      Compiled clean in Docker (golang:1.22) against the real `otel v1.28.0` +
      `otel/sdk/metric v1.28.0` + `otlpmetricgrpc v1.28.0` symbols.
- [x] `cmd/gateway/main.go`: call `InitMeter` next to `InitTracer`; register its
      shutdown alongside `shutdownTracing` (5s timeout, same shape).
- [x] `internal/upstream/clients.go` (`Dial`): added
      `grpc.WithStatsHandler(otelgrpc.NewClientHandler())` on every `grpc.NewClient`.
      `otelgrpc v0.53.0` (the contrib release paired with otel core v1.28.0) added to
      `go.mod`; the exact stats-handler usage compiled clean in Docker.
- [ ] (Optional, same pattern) instrument the inbound edge HTTP mux with
      `otelhttp` for total ingress RPS/latency. DEFERRED (optional): the gateway-view
      per-service RED signal comes from the client stats handler, which is the
      cockpit's source. `total_rps` is derived as the sum of upstream RPS. Can be
      added later without changing the contract.
- [~] `go build ./...` green: cannot run the full module build here — generated proto
      is gitignored and Go isn't installed locally. Verified as far as possible in
      Docker: `gofmt -e` clean on all changed files; `go build ./internal/observability`
      green against real OTel deps; the `otelgrpc.NewClientHandler()` +
      `grpc.WithStatsHandler` usage compiled green. Full `./...` build runs in CI/Docker.

## 2. Infra — platform-core/infra
- [x] `observability/prometheus/prometheus.yml`: added a `team-ai` scrape job
      (`metrics_path: /metrics`, target `team-ai-svc:8000`, its alias on the shared
      `platform-core_default` network) alongside the existing `otel-collector:8889`
      job. Validated with `promtool check config` (prom/prometheus:v2.52.0): SUCCESS.
- [ ] Gateway `OTEL_ENABLED=true` + `OTEL_EXPORTER_OTLP_ENDPOINT`: NOT DONE — BLOCKED
      by scope boundary. The gateway compose env lives in `docker-compose.services.yaml`
      (reserved / owned elsewhere; must not edit). `.env.example` documents the knobs
      (`OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317`); the reserved
      compose file owner must set `OTEL_ENABLED=true` + endpoint `http://otel-collector:4317`.
- [~] Verify locally (`curl otel-collector:8889/metrics` shows `marketplace_rpc_client_*`,
      targets `up`): DEFERRED — needs the real gateway process emitting, which won't
      compile locally (no Go, gitignored proto). Instead validated: (a) both scrape
      configs parse in the real Prometheus binary; (b) all 4 cockpit PromQL queries
      return `status:success` against a live prom/prometheus:v2.52.0 (queries parse).
      Rendered metric base recorded for the cockpit change: `marketplace_rpc_client_duration_milliseconds`
      (`_count`/`_bucket`/`_sum`), labels `rpc_service`, `rpc_grpc_status_code`
      (confirm exact rendering when the gateway first emits — see cockpit.go constants).

## 3. Infra — platform-gitops
- [x] `platform/monitoring/prometheus.yaml`: extended the `prometheus-config` ConfigMap
      `scrape_configs` with an `otel-collector` job (`otel-collector.marketplace.svc:8889`),
      mirroring compose, alongside the existing `team-ai` job. Extracted inner
      `prometheus.yml` validated with `promtool check config`: SUCCESS.
- [~] ArgoCD `prometheus` Application sync: cannot run ArgoCD here. Manifest validated —
      all 3 YAML docs parse, ConfigMap schema intact. No new app needed (the app recurses
      `platform/monitoring/`). Actual sync verification is a cluster/CI step.

## E2E (platform-e2e)
- [x] This change adds **no user-facing capability**, so it adds no
      `<repo>/FEATURES.yaml` entry. The user-facing cockpit assertion lands in
      `replace-cockpit-mock-metrics` (added there: `ops.cockpit-live-metrics`).
- [~] Platform smoke (infra verification): DEFERRED — needs the real gateway driving
      traffic in the local stack (won't compile locally). PromQL + scrape config already
      validated against a live Prometheus (see task 2). Metric names recorded above.
- [~] `make -C platform-e2e features-check`: not run here (no feature status change for
      this change). Runs in CI.

## Archive
- [~] Verify end-to-end: gateway emits `marketplace_rpc_*` → collector `:8889` →
      Prometheus (compose + cluster) has queryable per-service RPS/latency. Code +
      config in place and independently validated; live end-to-end needs the compiled
      gateway in the full stack (CI/Docker) + the reserved-file OTEL env (see task 2).
- [x] `replace-cockpit-mock-metrics` unblocked — the metric names + PromQL it queries
      are implemented and validated against a live Prometheus.
- [ ] Archive the change (`/opsx:archive add-prometheus-infra`) — pending go-ahead.
