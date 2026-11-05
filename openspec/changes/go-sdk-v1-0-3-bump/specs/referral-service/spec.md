## MODIFIED Requirements

### Requirement: The service follows the ADR-0016 layout and the shared build files

team-referral SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once in `go.mod`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory, and SHALL pin golangci-lint as a `tool` directive. `cmd/server/main.go` SHALL only load settings and call `app.Run(ctx, settings)`. `internal/app.Run` SHALL build, in order, the logger (`logx`), telemetry (`otelx`), health (`healthx`), the Postgres pool with its health check, the gRPC server (`grpcx.NewServer` with the SDK interceptor chain) and the lifecycle (`lifecycle.Run`), and SHALL close what it opened in reverse order on shutdown and on an early failure. The `Makefile` SHALL include `../platform-core/make/go-service.mk`, and the service SHALL NOT contain its own `.golangci.yml` or a copy of `go-service.mk`.

#### Scenario: The module requires the tag without a replace
- **WHEN** `team-referral/go.mod` is read
- **THEN** it requires go-sdk exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has a golangci-lint `tool` line and contains no `replace` line, and `team-referral/third_party` does not exist
- **VERIFIED BY**: `team-referral/internal/config` `TestModuleRequiresSDKTagWithoutReplace`

#### Scenario: The Makefile uses the shared files
- **WHEN** `team-referral/Makefile` is read
- **THEN** it includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and `team-referral/.golangci.yml` and `team-referral/make/` do not exist
- **VERIFIED BY**: `team-referral/internal/config` `TestMakefileUsesSharedFiles`

#### Scenario: A failed start releases what was opened
- **WHEN** `app.Run` fails after the pool opened (for example the gRPC port is already bound)
- **THEN** it returns the error and the pool is closed, so a following `app.Run` on the same database starts
- **VERIFIED BY**: `team-referral/internal/app` `TestEarlyFailureClosesPool`

#### Scenario: Context cancellation stops the server within the grace period
- **WHEN** the context passed to `app.Run` is cancelled while a call is in flight and `SHUTDOWN_GRACE=2s`
- **THEN** `app.Run` returns within 3 seconds and the listener no longer accepts connections
- **VERIFIED BY**: `team-referral/internal/app` `TestShutdownWithinGrace`

#### Scenario: The service answers on its port after the migration
- **WHEN** the stack runs with the migrated image and a buyer creates a referral code through the gateway
- **THEN** the call succeeds and the container is running, has not restarted and carries no removed flag; the service has no Docker healthcheck, and its gRPC health is verified by `TestHealthReportsDatabaseDown` and the other health scenarios
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/services/new_services.feature` scenario "The service answers on its port after the migration"
