## ADDED Requirements

### Requirement: team-gateway uses go-sdk platform parts with explicit wiring

team-gateway SHALL wire go-sdk v0.2.1 `config`, `logx`, `otelx`, `lifecycle` and `kafkax` in an `internal/app` composition root. `cmd/gateway/main.go` SHALL only create the signal context, load settings and call `app.Run`. Real toggles (`EDGE_REFLECTION_ENABLED`, `OTEL_ENABLED`) SHALL stay, and reflection SHALL NOT be enabled in a strict ENV.

#### Scenario: Reflection stays rejected in production
- **WHEN** the gateway loads settings with `ENV=production` and `EDGE_REFLECTION_ENABLED=true`
- **THEN** loading fails with an error naming `EDGE_REFLECTION_ENABLED`
- **VERIFIED BY**: `team-gateway/internal/config` `TestReflectionRejectedInProd`

#### Scenario: Shutdown drains in-flight requests
- **WHEN** a slow request is in flight and the gateway context is cancelled
- **THEN** the request completes before the HTTP server stops, and new connections are not accepted
- **VERIFIED BY**: `team-gateway/internal/app` `TestShutdownDrainsInFlightRequest`

### Requirement: team-gateway behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except removed-flag fields.

#### Scenario: team-gateway existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-gateway` `make check`
