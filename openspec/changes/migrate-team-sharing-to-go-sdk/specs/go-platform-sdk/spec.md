## MODIFIED Requirements

### Requirement: The example service is the reference wiring

`packages/go-sdk/examples/hello` SHALL wire `config`, `logx`, `otelx`, `pgx` (optional), `healthx`, `auth`, `grpcx` and `lifecycle` in an `internal/app.Run(ctx context.Context, s config.Settings) error` composition root (`main` loads settings), with a short `cmd/hello/main.go` that only creates the signal context, loads settings and calls `app.Run`. Migrations SHALL live in the example's root `migrations/` directory and SHALL NOT be applied at boot; an unreachable database outside `ENV=local` SHALL fail boot, and in `ENV=local` MAY be tolerated. SIGTERM SHALL drain health before the server stops.

#### Scenario: Health drains before stop
- **WHEN** the example runs and its context is cancelled
- **THEN** a health watcher observes NOT_SERVING before the listener closes
- **VERIFIED BY**: `platform-core/packages/go-sdk/examples/hello/internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: Calls without a principal are rejected
- **WHEN** `Greet` is called without `x-principal-*` metadata
- **THEN** the caller receives `Unauthenticated`
- **VERIFIED BY**: `TestGreetRejectsMissingPrincipal`
