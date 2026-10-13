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

### Requirement: Settings load from environment with the services' existing tags

`config.Load[T]()` SHALL populate a settings struct from environment variables. It reads `env:"NAME"` and `default:"value"` tags, the tag names the services use today, and recurses into nested structs without prefixing. It SHALL return an error naming every variable that fails to parse or is marked `required` and unset. After parsing, when `T` implements `Validate() error`, `Load` SHALL call it and return its error. `config.Postgres.Validate()` SHALL reject an empty `URL`; the database is always on, with no enable flag. The SDK SHALL provide the embeddable groups `Runtime`, `GRPC`, `Postgres` and `OTel`. Their variable names and defaults SHALL be those already used by the services, minus enable flags for capabilities every deploy runs: `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT`, `SHUTDOWN_GRACE_SECONDS`, `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.

#### Scenario: Defaults apply and env overrides them
- **WHEN** a struct embedding `config.Runtime` and `config.GRPC` is loaded with only `GRPC_PORT=50065` and `LOG_LEVEL=debug` set
- **THEN** `GRPC.Port` is 50065, `Runtime.LogLevel` is `debug`, `GRPC.Host` is `0.0.0.0` and `GRPC.ShutdownGrace()` is 10s
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadDefaultsAndOverrides`

#### Scenario: Unparseable values fail with the variable name
- **WHEN** `GRPC_PORT=abc` is set
- **THEN** `Load` returns an error whose message contains `GRPC_PORT`
- **VERIFIED BY**: `TestLoadReportsBadVariable`

#### Scenario: Enabled database without a URL fails validation
- **WHEN** a settings struct whose `Validate() error` delegates to `config.Postgres.Validate()` is loaded without `DATABASE_URL`
- **THEN** `Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: `TestLoadRunsValidate`

### Requirement: gRPC servers share one interceptor chain and safe errors

`grpcx.NewServer(opts)` SHALL return a `*grpc.Server` with:
- the otelgrpc stats handler;
- the unary and stream chain request-id → recovery → logging → auth (request-id outermost so panic logs carry it);
- the health service from `opts.Health`;
- the reflection service, always (no flag).

The request-id interceptor SHALL reuse an incoming `x-request-id` or generate one, store it in `reqctx` and send it back as a response header. `grpcx.Serve(srv, cfg config.GRPC)` SHALL return a lifecycle component that listens on `cfg.Addr()` and stops gracefully when its context ends. `grpcx.ServeListener(srv, lis, grace)` SHALL do the same on a caller-provided listener, for tests. `grpcx.Status(err, table)` SHALL map errors matched by `errors.Is` to their code. An error that already carries a gRPC status other than `Unknown` (for example from `auth.RequirePrincipal`) SHALL be returned unchanged. Any other error SHALL be logged and mapped to `codes.Internal` with the message `internal error`.

#### Scenario: A panicking handler returns Internal and the server keeps serving
- **WHEN** a handler panics
- **THEN** the caller receives `codes.Internal` and a following call succeeds
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestRecoveryKeepsServing`

#### Scenario: Request id round-trips
- **WHEN** a call sends `x-request-id=abc`
- **THEN** the handler sees `reqctx.RequestID(ctx)=="abc"` and the response header carries `abc`
- **VERIFIED BY**: `TestRequestIDPropagates`

#### Scenario: Unknown errors do not leak
- **WHEN** a handler returns `grpcx.Status(errors.New("pq: password=secret"), table)`
- **THEN** the caller sees `Internal` with message `internal error`
- **VERIFIED BY**: `TestStatusHidesUnknownErrors`

#### Scenario: Existing gRPC status errors pass through
- **WHEN** a handler returns `grpcx.Status(status.Error(codes.PermissionDenied, "missing scope"), table)`
- **THEN** the caller sees `PermissionDenied` with message `missing scope`, not `Internal`
- **VERIFIED BY**: `TestStatusPassesThroughGRPCStatus`
