## Context

Prometheus, the otel-collector (with an OTLP→Prometheus metrics pipeline on
`:8889`, namespace `marketplace`), Jaeger and Grafana already run in
`platform-core/infra/docker-compose.yaml`, and an in-cluster Prometheus already
ships via `platform-gitops`. What is missing is the **producer side**: no Go
service emits request (RED) metrics — they wire OTel tracing only. The one design
question worth an ADR-lite note is *how* services should expose those metrics.

## Decision — emit OTel metrics via the existing collector→Prometheus bridge (Option A), gateway-centric

Instrument the **gateway** with an OpenTelemetry `MeterProvider` + `otelgrpc`
client instrumentation and let the metrics reach Prometheus through the
otel-collector's already-configured `:8889` exporter.

### Options considered

- **A — OTel `MeterProvider` → otel-collector → Prometheus `:8889` (chosen).**
  Add `InitMeter` next to `InitTracer` (identical config-swappable, OFF-by-default
  shape) and attach `otelgrpc.NewClientHandler()` at the single upstream dial site
  `upstream.Dial` (`grpc.NewClient(addr, grpc.WithStatsHandler(...))`). The gateway
  is the one edge that dials every service (rules 1 & 2), so its **client-side**
  gRPC metrics already carry per-upstream request-count and duration histograms,
  labelled by RPC service/method — per-service RPS/latency from **one** service.
  Reuses infra that already exists (collector pipeline + Prometheus scrape of
  `:8889`); adds **zero** new scrape targets and touches **one** repo.
- **B — per-service `promhttp` `/metrics` + one scrape job per service (rejected).**
  Add `client_golang` + a `/metrics` HTTP listener to each of the 9 gRPC-only Go
  services and 9 static scrape jobs. More invasive: a new dependency in 9 repos, a
  new HTTP port on gRPC-only services, 9 scrape targets to maintain, and it
  **duplicates** the OTel path ADR-0004 already standardises on.

### Why A

- **ADR-0004 compliance.** ADR-0004 mandates OpenTelemetry with a swappable
  exporter ("swap the backend by changing config, not code"). Option A is exactly
  that pattern extended from traces to metrics; Option B forks a second,
  non-OTel metrics path.
- **Least invasive.** One meter init + one stats-handler line at one dial site,
  versus a new listener and dependency in 9 repos plus 9 scrape jobs.
- **Rules 1 & 2.** The edge already sees all traffic and already holds the routing
  table; sourcing per-service RED metrics from its client instrumentation adds no
  business logic and no new surface.
- **team-ai already fits.** Its existing Prometheus-text `/metrics` is added as a
  single direct scrape job (config-only) so its in-service counter is also
  captured; no code change there.

### Consequences / trade-offs

- Per-service metrics are the **gateway's view** of each upstream (client-side
  latency includes the gateway↔service hop, not in-service processing only). That
  is the correct RED signal for an edge cockpit and matches what the HUD shows.
  Deeper per-service server-side metrics can be added later, per service, without
  changing this contract — a later change, not this one.
- Metric series names follow OTel gRPC semconv rendered through the collector's
  Prometheus exporter under the `marketplace_*` namespace (e.g.
  `marketplace_rpc_client_duration_milliseconds_*`); the exact rendered names
  settle at apply time and are the source the cockpit change queries.
- Enabling the meter requires `OTEL_ENABLED=true` + an OTLP endpoint for the
  gateway in compose; default-off elsewhere keeps local dev free (ADR-0004).
