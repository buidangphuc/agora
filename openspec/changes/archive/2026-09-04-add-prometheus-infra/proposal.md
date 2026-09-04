## Why

The Admin Cockpit HUD (`/admin/cockpit`) reads per-service RPS/latency/error-rate
from the gateway, but the gateway currently fabricates them with `math/rand`
(`team-gateway/internal/edge/cockpit.go`). There is no real metrics source to
query, so no value the HUD shows is true.

The observability infra is **already stood up** — it just is not fed real service
request metrics:

- `platform-core/infra/docker-compose.yaml` already runs `prometheus`
  (`prom/prometheus:v2.52.0`, `:9090`), `otel-collector` (OTLP in on `:4317/4318`,
  Prometheus exporter out on `:8889`, namespace `marketplace`), `jaeger`, and
  `grafana`. The collector's OTLP→Prometheus **metrics pipeline already exists**
  (`platform-core/infra/otel-collector.yaml`) and Prometheus already scrapes the
  collector (`platform-core/infra/observability/prometheus/prometheus.yml`).
- `platform-gitops/argocd/apps/prometheus.yaml` already deploys an in-cluster
  Prometheus (`platform-gitops/platform/monitoring/prometheus.yaml`).

The gap is upstream of Prometheus: **no Go service emits request metrics.** Every
Go service (gateway included) wires OTel **tracing only** — `InitTracer` in
`team-gateway/internal/observability/tracer.go` installs an `sdktrace` provider
and an OTLP **trace** exporter; there is no `MeterProvider` and no RED (Rate/
Errors/Duration) instrumentation. `OTEL_ENABLED` defaults to `false`. The one
exception is `team-ai`, which already exposes a hand-rolled Prometheus text
`/metrics` (`team-ai/app/core/grpc_metrics.py`, route in
`team-ai/app/api/v1/health/router.py`) for KEDA autoscaling — but the in-cluster
Prometheus only scrapes that one target and the compose Prometheus does not scrape
it at all.

This change closes that gap: emit real per-service request metrics and route them
into the Prometheus that already exists, so change `replace-cockpit-mock-metrics`
has real RPS/latency to query.

## What Changes

- **team-gateway (`internal/observability`, `internal/upstream`)** — add an
  `InitMeter` alongside the existing `InitTracer` (same OTLP-exporter, config-
  swappable, no-op when `OTEL_ENABLED=false` shape), and instrument the edge with
  OpenTelemetry gRPC/HTTP RED metrics. Because the gateway is the single edge that
  dials **every** upstream (rules 1 & 2), attach `otelgrpc` client instrumentation
  at the one dial site (`upstream.Dial` → `grpc.NewClient(addr, grpc.WithStatsHandler(...))`)
  so per-upstream request-count + duration histograms are emitted from **one
  service**, labelled by target service — no other Go service is touched and no new
  scrape target is added. Metrics flow gateway → otel-collector (OTLP) →
  Prometheus exporter `:8889` → Prometheus (already scraped). See `design.md` for
  the A-vs-B decision.
- **platform-core/infra** — add a `team-ai` scrape job to the compose
  `observability/prometheus/prometheus.yml` (team-ai already serves Prometheus-text
  `/metrics`; this is config-only, no code) so its request counter is captured
  locally too. Confirm the collector metrics pipeline surfaces the gateway RED
  series under the `marketplace_*` namespace. Ensure the gateway compose service
  sets `OTEL_ENABLED=true` + `OTEL_EXPORTER_OTLP_ENDPOINT` at the collector so the
  new meter actually exports.
- **platform-gitops** — extend the in-cluster Prometheus scrape ConfigMap
  (`platform/monitoring/prometheus.yaml`) to also scrape `otel-collector:8889`
  (mirroring compose) so the cluster Prometheus carries the gateway RED series, not
  only team-ai's `/metrics`. No new ArgoCD app is needed — `prometheus.yaml` app
  already syncs `platform/monitoring/` recursively.

## Non-goals

- Grafana dashboards or panels (the datasource is already provisioned; building
  dashboards is out of scope).
- Prometheus alerting/recording rules.
- Replacing or altering OTel **traces** (Jaeger) — tracing stays as-is (ADR-0004).
- Per-service `promhttp` `/metrics` endpoints on the other 9 Go services, and
  9 new scrape jobs (rejected in `design.md` as the more invasive path).
- A real order-domain business metric (orders/revenue counters) — those numbers
  stay derived; consuming them is scoped in `replace-cockpit-mock-metrics`.
- Any change to `team-ai`'s existing `/metrics` (it already works).
