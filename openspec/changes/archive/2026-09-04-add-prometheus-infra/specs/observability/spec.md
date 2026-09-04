## ADDED Requirements

### Requirement: The gateway emits real per-service request metrics via OpenTelemetry

The gateway SHALL emit OpenTelemetry RED metrics (request count and request
duration histograms) for every upstream service it calls, labelled by upstream
service, using a `MeterProvider` wired the same config-swappable, OFF-by-default
way as the existing tracer (ADR-0004). The metrics SHALL be exported over OTLP to
the otel-collector and require no per-service `promhttp` `/metrics` endpoint on the
other services. When observability is disabled (`OTEL_ENABLED=false`) the meter
SHALL be a no-op and add no runtime cost.

#### Scenario: Driving traffic through the gateway produces per-service metrics

- **WHEN** requests are routed through the gateway to upstream services (e.g.
  search and listing reads) with `OTEL_ENABLED=true`
- **THEN** the gateway emits request-count and duration-histogram metrics tagged
  with each upstream service, exported over OTLP to the otel-collector

#### Scenario: Metrics are disabled by default in local dev

- **WHEN** the gateway starts with `OTEL_ENABLED=false`
- **THEN** the meter provider is a no-op, no metrics are exported, and startup and
  request handling are unaffected

### Requirement: Prometheus scrapes real service metrics

Prometheus SHALL scrape real service request metrics so that per-service RPS,
latency and error-rate can be queried. In compose, Prometheus SHALL obtain the
gateway RED series from the otel-collector Prometheus exporter (`:8889`) and SHALL
additionally scrape `team-ai`'s existing Prometheus-text `/metrics`. In-cluster,
the Prometheus scrape config SHALL include the otel-collector exporter in addition
to the existing `team-ai` target.

#### Scenario: Prometheus exposes queryable service RPS after traffic

- **WHEN** traffic has been driven through the gateway and Prometheus has scraped
  at least one interval
- **THEN** a PromQL query for per-service request rate over the gateway RED series
  returns non-zero samples for the exercised services

#### Scenario: team-ai metrics remain scraped

- **WHEN** the compose and in-cluster Prometheus configs are applied
- **THEN** `team-ai`'s `/metrics` endpoint is a configured scrape target in both,
  and its request counter is queryable

### Requirement: Tracing is unchanged

This change SHALL NOT alter OpenTelemetry tracing or the Jaeger pipeline; the trace
exporter, propagation, and span behaviour remain as defined by ADR-0004.

#### Scenario: Traces still flow to Jaeger

- **WHEN** the gateway runs with observability enabled after this change
- **THEN** traces are still exported to the collector and visible in Jaeger,
  unchanged from before
