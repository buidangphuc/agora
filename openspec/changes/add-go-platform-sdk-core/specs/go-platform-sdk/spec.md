## ADDED Requirements

### Requirement: Settings load from environment with the services' existing tags

`config.Load[T]()` SHALL populate a settings struct from environment variables. It reads `env:"NAME"` and `default:"value"` tags, the tag names the services use today, and recurses into nested structs without prefixing. It SHALL return an error naming every variable that fails to parse or is marked `required` and unset. After parsing, when `T` implements `Validate() error`, `Load` SHALL call it and return its error. `config.Postgres.Validate()` SHALL reject `Enabled` with an empty `URL`. The SDK SHALL provide the embeddable groups `Runtime`, `GRPC`, `Postgres` and `OTel`. Their variable names and defaults SHALL be those already used by the services: `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT`, `GRPC_REFLECTION_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `DATABASE_ENABLED`, `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.

#### Scenario: Defaults apply and env overrides them
- **WHEN** a struct embedding `config.Runtime` and `config.GRPC` is loaded with only `GRPC_PORT=50065` and `LOG_LEVEL=debug` set
- **THEN** `GRPC.Port` is 50065, `Runtime.LogLevel` is `debug`, `GRPC.Host` is `0.0.0.0` and `GRPC.ShutdownGrace()` is 10s
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadDefaultsAndOverrides`

#### Scenario: Unparseable values fail with the variable name
- **WHEN** `GRPC_PORT=abc` is set
- **THEN** `Load` returns an error whose message contains `GRPC_PORT`
- **VERIFIED BY**: `TestLoadReportsBadVariable`

#### Scenario: Enabled database without a URL fails validation
- **WHEN** a settings struct whose `Validate() error` delegates to `config.Postgres.Validate()` is loaded with `DATABASE_ENABLED=true` and no `DATABASE_URL`
- **THEN** `Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: `TestLoadRunsValidate`

### Requirement: .env.example drift is detectable

`config.CheckEnvExample(path, v)` SHALL compare the keys assigned in the `.env.example` file at `path` with the env keys declared by the struct `v`, in both directions. It SHALL return an error listing missing and stale keys, sorted.

#### Scenario: Missing and stale keys are both reported
- **WHEN** the struct declares `A` and `B` and the file assigns `B` and `C`
- **THEN** the error lists `A` as missing and `C` as stale
- **VERIFIED BY**: `TestCheckEnvExampleReportsBothDirections`

### Requirement: Logs carry request, principal and trace identifiers

`logx.New(runtime config.Runtime, service string)` SHALL return an `*slog.Logger` using JSON when `LOG_JSON=true` and text otherwise, at the level from `LOG_LEVEL`, with a `service` attribute. Records logged with a context SHALL include `request_id`, `principal_id` and `trace_id` when the context carries them.

#### Scenario: Context identifiers appear in log records
- **WHEN** a record is logged with a context holding request id `r1`, principal id `u1` and an active span
- **THEN** the JSON record has `request_id=r1`, `principal_id=u1` and the span's `trace_id`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/logx` `TestHandlerAddsContextIDs`

#### Scenario: Missing identifiers are omitted, not logged empty
- **WHEN** a record is logged with a context holding no request id, principal or span
- **THEN** the record has no `request_id`, `principal_id` or `trace_id` keys
- **VERIFIED BY**: `TestHandlerOmitsMissingIDs`

### Requirement: Telemetry is a no-op unless enabled

`otelx.Init(ctx, cfg config.OTel, service string)` SHALL always install the W3C trace-context propagator. With `OTEL_ENABLED=false` it SHALL install no exporter and return a no-op shutdown. With `OTEL_ENABLED=true` it SHALL install OTLP/gRPC tracer and meter providers with the service name, falling back to `service` when `OTEL_SERVICE_NAME` is empty. It SHALL return a shutdown that flushes both providers.

#### Scenario: Disabled telemetry exports nothing
- **WHEN** `Init` runs with `Enabled=false`
- **THEN** the global tracer provider is not an SDK provider and shutdown returns nil
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/otelx` `TestInitDisabledIsNoop`

#### Scenario: Enabled telemetry names the service
- **WHEN** `Init` runs with `Enabled=true`, an empty `ServiceName` and `service="team-x"`
- **THEN** spans carry resource attribute `service.name=team-x`
- **VERIFIED BY**: `TestInitEnabledUsesServiceFallback`

### Requirement: Principal is read from gateway metadata with exact scope checks

`auth` SHALL read the principal from incoming metadata `x-principal-id`, `x-principal-type` and `x-principal-scopes` (comma-separated) in unary and stream interceptors. It SHALL store the principal in the context and set `reqctx` principal id. `RequirePrincipal(ctx)` SHALL return `codes.Unauthenticated` when the principal is missing, has id `anonymous` or has type `anonymous`. `RequireScopes(ctx, scopes...)` SHALL return `codes.PermissionDenied` unless the principal holds every listed scope by exact match. The `admin` scope SHALL grant nothing implicitly.

#### Scenario: Anonymous principal is unauthenticated
- **WHEN** metadata carries `x-principal-id=anonymous`
- **THEN** `RequirePrincipal` returns `Unauthenticated`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auth` `TestRequirePrincipalRejectsAnonymous`

#### Scenario: Admin is not a wildcard
- **WHEN** the principal holds only scope `admin` and `RequireScopes(ctx, "order.admin")` is called
- **THEN** it returns `PermissionDenied`
- **VERIFIED BY**: `TestRequireScopesExactMatchNoAdminBypass`

#### Scenario: Stream handlers see the principal
- **WHEN** a streaming RPC is called with principal metadata
- **THEN** the handler's stream context returns that principal
- **VERIFIED BY**: `TestStreamInterceptorInjectsPrincipal`

### Requirement: Health reflects dependency checks and drains on shutdown

`healthx.New(interval)` SHALL wrap the gRPC health server. `Register(name, check func(ctx) error)` SHALL add checkers. `Run(ctx)` SHALL poll all checkers every interval, each bounded by the interval, and set the overall status to SERVING when all pass and NOT_SERVING when any fails. It SHALL log state transitions only. `Drain()` SHALL set NOT_SERVING permanently.

#### Scenario: A failing checker flips status
- **WHEN** a registered checker starts failing
- **THEN** within one interval the overall status is NOT_SERVING, and it returns to SERVING after the checker passes again
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/healthx` `TestCheckerFailureFlipsStatus`

#### Scenario: Drain wins over passing checks
- **WHEN** `Drain()` is called while all checkers pass
- **THEN** status stays NOT_SERVING
- **VERIFIED BY**: `TestDrainIsSticky`

#### Scenario: A hanging checker counts as failed
- **WHEN** a checker blocks longer than the interval
- **THEN** it is cancelled, the status becomes NOT_SERVING, and other checkers still run every interval
- **VERIFIED BY**: `TestHangingCheckerFailsWithoutBlocking`

### Requirement: Components run and stop as one unit

`lifecycle.Run(ctx, opts, components...)` SHALL start every component's `Run(ctx) error` in an errgroup. It SHALL return when ctx is cancelled or any component returns. On return it SHALL first call the `OnShutdown` hooks (for example health drain), then cancel the components, wait for them up to `ShutdownGrace`, then call the closers in reverse registration order. It SHALL return the first non-context error. `Options.DrainDelay` (default 0, from `SHUTDOWN_DRAIN_DELAY_SECONDS` via `config.GRPC.DrainDelay()`) SHALL be waited after the hooks and before cancelling components, so probes observe NOT_SERVING. `lifecycle.Stack` SHALL collect closers during wiring: `Push(f)` registers, `Close(ctx)` runs them once in reverse order and joins errors, so a composition root can release what it opened when a later step fails.

#### Scenario: A failing component stops the others
- **WHEN** one of two components returns an error
- **THEN** the other component's context is cancelled and `Run` returns that error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/lifecycle` `TestFailingComponentCancelsOthers`

#### Scenario: Closers run in reverse order after drain
- **WHEN** ctx is cancelled with closers `a` then `b` registered and an `OnShutdown` hook
- **THEN** the hook runs first, then `b`, then `a`
- **VERIFIED BY**: `TestShutdownOrder`

#### Scenario: Shutdown is bounded
- **WHEN** a component ignores cancellation longer than `ShutdownGrace`
- **THEN** `Run` returns after the grace period with a deadline error
- **VERIFIED BY**: `TestShutdownDeadline`

#### Scenario: Stack closes in reverse order exactly once
- **WHEN** closers `a`, `b` are pushed, `b` fails, and `Close` is called twice
- **THEN** `b` then `a` run once each and the first `Close` returns `b`'s error
- **VERIFIED BY**: `TestStackClosesReverseOnce`

#### Scenario: Drain delay holds components after the hooks
- **WHEN** `DrainDelay` is 100ms and ctx is cancelled
- **THEN** components see cancellation no earlier than 100ms after the hooks ran
- **VERIFIED BY**: `TestDrainDelayHoldsComponents`

### Requirement: Postgres pool fails fast and is probed

`pgx.Open(ctx, cfg config.Postgres)` SHALL parse the URL, apply `MaxConns`, create the pool and ping it within 5s. It SHALL return an error if the ping fails. `pgx.Checker(pool)` SHALL return a health check that pings. `pgx.WithTx(ctx, pool, fn)` SHALL commit when `fn` returns nil and roll back otherwise.

#### Scenario: Unreachable database fails at open
- **WHEN** `Open` targets a closed port
- **THEN** it returns an error before any query runs
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/pgx` `TestOpenFailsOnUnreachableDB`

#### Scenario: WithTx rolls back on error
- **WHEN** `fn` inserts a row and returns an error
- **THEN** the row is not visible afterwards
- **VERIFIED BY**: `TestWithTxRollsBackOnError` (testkit Postgres)

### Requirement: Migrations run from a service's migrations directory

`migrate.Up(ctx, databaseURL, fsys fs.FS)` SHALL apply golang-migrate `NNN_name.up.sql` files from `fsys` and treat "no change" as success.

#### Scenario: Up is idempotent
- **WHEN** `Up` runs twice on the same database
- **THEN** both calls succeed and the schema version is the latest
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/migrate` `TestUpIsIdempotent`

#### Scenario: Invalid SQL fails the migration
- **WHEN** an up file contains invalid SQL
- **THEN** `Up` returns an error that names the failing version
- **VERIFIED BY**: `TestUpFailsOnInvalidSQL`

### Requirement: Retries and circuit breaking use shared policies

`resilience` SHALL expose `Retry(opts)` and `Breaker(opts)` built on failsafe-go, plus `Do(ctx, fn, policies...)`. Retries SHALL use exponential backoff with jitter and SHALL NOT retry errors whose gRPC code is `InvalidArgument`, `NotFound`, `AlreadyExists`, `PermissionDenied`, `Unauthenticated` or `FailedPrecondition`.

#### Scenario: Non-retryable codes are not retried
- **WHEN** `fn` returns `status.Error(codes.InvalidArgument, …)` under a 3-attempt retry
- **THEN** `fn` runs once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/resilience` `TestRetrySkipsNonRetryableCodes`

#### Scenario: Open breaker rejects calls
- **WHEN** failures exceed the breaker threshold
- **THEN** the next call returns the breaker-open error without running `fn`
- **VERIFIED BY**: `TestBreakerOpensAfterFailures`

### Requirement: gRPC servers share one interceptor chain and safe errors

`grpcx.NewServer(opts)` SHALL return a `*grpc.Server` with:
- the otelgrpc stats handler;
- the unary and stream chain request-id → recovery → logging → auth (request-id outermost so panic logs carry it);
- the health service from `opts.Health`;
- reflection when `GRPC_REFLECTION_ENABLED`.

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

### Requirement: The example service is the reference wiring

`packages/go-sdk/examples/hello` SHALL wire `config`, `logx`, `otelx`, `pgx` (optional), `healthx`, `auth`, `grpcx` and `lifecycle` in an `internal/app.Run(ctx context.Context, s config.Settings) error` composition root (`main` loads settings), with a short `cmd/hello/main.go` that only creates the signal context, loads settings and calls `app.Run`. A migration failure SHALL fail boot in every environment; only an unreachable database in `ENV=local` MAY be tolerated. SIGTERM SHALL drain health before the server stops.

#### Scenario: Health drains before stop
- **WHEN** the example runs and its context is cancelled
- **THEN** a health watcher observes NOT_SERVING before the listener closes
- **VERIFIED BY**: `platform-core/packages/go-sdk/examples/hello/internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: Calls without a principal are rejected
- **WHEN** `Greet` is called without `x-principal-*` metadata
- **THEN** the caller receives `Unauthenticated`
- **VERIFIED BY**: `TestGreetRejectsMissingPrincipal`

### Requirement: Services vendor a pinned SDK copy

`platform-core/tools/sync-go-sdk.sh <tag> <service-dir>...` SHALL, for each service directory:
- export `packages/go-sdk` at git tag `<tag>` into `<service>/third_party/go-sdk` (replacing any previous copy);
- write `third_party/go-sdk/VERSION` containing the tag and commit;
- copy `make/go-service.mk` and `.golangci.yml` into the service;
- ensure `go.mod` has `replace github.com/buidangphuc/platform-core/packages/go-sdk => ./third_party/go-sdk`;
- run `go mod tidy`.

It SHALL fail without changes if the tag does not exist.

#### Scenario: Sync vendors the tagged tree
- **WHEN** the script runs with an existing tag against a scratch copy of a service
- **THEN** `third_party/go-sdk/VERSION` names the tag, `go.mod` has the replace, and `go build ./...` succeeds in the service
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`

#### Scenario: Unknown tag changes nothing
- **WHEN** the script runs with a tag that does not exist
- **THEN** it exits non-zero and the service tree is unchanged
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`
