## MODIFIED Requirements

### Requirement: Logs and telemetry use the SDK and keep the edge.request fields

The gateway SHALL build its logger with `logx` and its tracer and meter with `otelx`. `OTEL_ENABLED=false` SHALL install no exporter. The `edge.request` line SHALL carry the fields `method`, `principal_kind`, `code`, `latency_ms`, `request_id` and `attempts`, SHALL carry `principal_fp` for a user or service principal, and SHALL gain `trace_id` from the context. It SHALL NOT carry the raw principal id under any key, including `principal` and `principal_id`. No log line SHALL contain a bearer token, a request body or a Kafka record key. The revocation gauges `gateway_revocation_consumer_up`, `gateway_revocation_consumer_lag` and `gateway_revocation_denylist_size` SHALL be registered on the global meter.

#### Scenario: The edge.request line keeps its fields
- **WHEN** one unary call is served with `X-Request-Id` "req-9"
- **THEN** the log line `edge.request` has `method`, `principal_kind`, `code`, `latency_ms`, `request_id` "req-9" and `attempts`, with `request_id` appearing once
- **VERIFIED BY**: team-gateway `internal/edge` `TestEdgeRequestLineKeepsFields`

#### Scenario: A token never reaches the logs
- **WHEN** a call with an invalid `Authorization: Bearer <token>` is served and refused as `unauthenticated`
- **THEN** no captured log line contains the token string
- **VERIFIED BY**: team-gateway `internal/edge` `TestBearerTokenIsNeverLogged`

#### Scenario: Telemetry is disabled by default
- **WHEN** `Run` starts with `OTEL_ENABLED` unset
- **THEN** no OTLP connection is attempted and startup is unaffected
- **VERIFIED BY**: team-gateway `internal/app` `TestOTelDisabledByDefault`

#### Scenario: The revocation gauges report the consumer state
- **WHEN** the consumer is down, then up with two denylist entries
- **THEN** the gauges read up 0 and size 0, then up 1 and size 2
- **VERIFIED BY**: team-gateway `internal/revocation` `TestRevocationGaugesReportConsumerState`
