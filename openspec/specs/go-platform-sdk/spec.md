# go-platform-sdk Specification

## Purpose
TBD - created by archiving change add-go-platform-sdk-core. Update Purpose after archive.

## Requirements

### Requirement: Settings load from environment with the services' existing tags

`config.Load[T]()` SHALL populate a settings struct from environment variables. It reads `env:"NAME"` and `default:"value"` tags, the tag names the services use today, and recurses into nested structs without prefixing. It SHALL return an error naming every variable that fails to parse or is marked `required` and unset. After parsing, `Load` SHALL call `Validate() error` on `T` when it has one, and on every nested or embedded group that has one, and SHALL return all their errors joined. `config.Postgres.Validate()` SHALL reject an empty `URL`: the database has no enable flag. `config.Runtime.Validate()` SHALL reject an empty `ENV`. The SDK SHALL provide the embeddable groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox` and `Flags`. Their variable names and defaults SHALL be `ENV` (required, no default), `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT` (required), `SHUTDOWN_GRACE` (`10s`), `SHUTDOWN_DRAIN_DELAY` (`0s`), `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `KAFKA_BROKERS` (no tag default), `OUTBOX_POLL_INTERVAL` (`1s`), `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK` (`60s`), `FEATURE_FLAGS_ENABLED`, `FLIPT_ADDR` (no tag default) and `FEATURE_FLAGS_EVAL_TIMEOUT` (`500ms`). Every duration SHALL be a `time.Duration` field read as a Go duration, and no key carries a unit suffix. `KAFKA_BROKERS`, and `FLIPT_ADDR` while `FEATURE_FLAGS_ENABLED` is true, SHALL be required when `Runtime.IsStrict()` and default to `localhost:9092` and `localhost:9000` otherwise. `Load` SHALL enforce this by calling `ResolveFor(Runtime)` on `config.Kafka` and `config.Flags` with the `Runtime` it finds in the settings struct (embedded or a named field; the zero `Runtime`, so the local behaviour, when there is none); a `Settings` type needs nothing beyond holding `Runtime` and the group. There is no `GRPC_REFLECTION_ENABLED` and no `DATABASE_ENABLED`. `Runtime.IsLocal()` SHALL be true only for an explicit `ENV` of `local` or `test`.

#### Scenario: Defaults apply and env overrides them
- **WHEN** a struct embedding `config.Runtime` and `config.GRPC` is loaded with only `ENV=local`, `GRPC_PORT=50065` and `LOG_LEVEL=debug` set
- **THEN** `GRPC.Port` is 50065, `Runtime.LogLevel` is `debug`, `GRPC.Host` is `0.0.0.0` and `GRPC.ShutdownGrace` is 10s
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadDefaultsAndOverrides`

#### Scenario: Unparseable values fail with the variable name
- **WHEN** `GRPC_PORT=abc` is set
- **THEN** `Load` returns an error whose message contains `GRPC_PORT`
- **VERIFIED BY**: `TestLoadReportsBadVariable`

#### Scenario: ENV is required
- **WHEN** `ENV` is unset, or set to the empty string
- **THEN** `Load` returns an error naming `ENV`, and `IsLocal()` is true only for `local` and `test`
- **VERIFIED BY**: `TestLoadRequiresEnv` and `TestIsLocalOnlyForExplicitLocalOrTest`

#### Scenario: Enabled database without a URL fails validation
- **WHEN** a settings struct embedding `config.Postgres` (always on, no enable flag) is loaded without `DATABASE_URL`
- **THEN** `Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: `TestLoadRunsValidate`

#### Scenario: Every embedded group is validated
- **WHEN** a struct embeds `GRPC` and `Postgres` and both are invalid
- **THEN** `Load` returns one error naming `GRPC_PORT` and `DATABASE_URL`
- **VERIFIED BY**: `TestLoadValidatesEveryEmbeddedGroup`

#### Scenario: Durations are Go durations with suffix-free keys
- **WHEN** `OUTBOX_CLAIM_LOCK=90s`, `FEATURE_FLAGS_EVAL_TIMEOUT=250ms` and `SHUTDOWN_DRAIN_DELAY=1500ms` are set, and again with none set
- **THEN** the fields hold 90s, 250ms and 1.5s, then the defaults 60s, 500ms and 0s
- **VERIFIED BY**: `TestMessagingGroupOverrides`, `TestMessagingGroupDefaults` and `TestDrainDelay`

#### Scenario: Deployed environments require the broker address
- **WHEN** `ENV=prod` and `KAFKA_BROKERS` is unset or empty, and again with `ENV=local`
- **THEN** `Load` returns an error naming `KAFKA_BROKERS` and not `FLIPT_ADDR`, and for `local` fills `localhost:9092`
- **VERIFIED BY**: `TestStrictEnvRequiresKafka`, `TestStrictEnvRejectsEmptyKafkaBrokers` and `TestLocalEnvKeepsKafkaDefault`

#### Scenario: Feature flags have no on/off switch
- **WHEN** settings with a `Flags` group load under `ENV=local` and `ENV=prod` without `FLIPT_ADDR`, and again with `FEATURE_FLAGS_ENABLED=false` set
- **THEN** `FliptAddr` is `localhost:9000` in both, the variable `FEATURE_FLAGS_ENABLED` is not read, and an unreachable Flipt only makes evaluations return the caller default
- **VERIFIED BY**: `TestFlagsAlwaysPointAtFlipt`, `TestMessagingGroupDefaults` and `TestUnreachableFliptReturnsDefault`

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

`healthx.New(interval, logger) (*Health, error)` SHALL wrap the gRPC health server and SHALL return an error when interval is not positive. `Register(name, check func(ctx) error)` SHALL add checkers. `Run(ctx)` SHALL poll all checkers every interval, each bounded by the interval, and set the overall status to SERVING when all pass and NOT_SERVING when any fails. It SHALL log state transitions only. `Drain()` SHALL set NOT_SERVING permanently.

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

#### Scenario: A non-positive interval is rejected
- **WHEN** `New` gets an interval of zero or less
- **THEN** it returns an error
- **VERIFIED BY**: `TestNewRejectsNonPositiveInterval`

### Requirement: Components run and stop as one unit

`lifecycle.Run(ctx, opts, components...)` SHALL start every component's `Run(ctx) error` in an errgroup. It SHALL return when ctx is cancelled or any component returns. On return it SHALL first call the `OnShutdown` hooks (for example health drain), then cancel the components, wait for them, then call the closers in reverse registration order; components and closers SHALL share one `ShutdownGrace` budget, so the worst case after the drain delay is one grace period. `Options.CloseGrace` SHALL reserve part of that budget for the closers: the components SHALL get `ShutdownGrace` minus `CloseGrace` to stop and the closers SHALL receive a context that is still live afterwards. Zero means a fifth of `ShutdownGrace`, at most 2s. It SHALL return the first non-context error. `Options.DrainDelay` (default 0, from `SHUTDOWN_DRAIN_DELAY` via `config.GRPC.DrainDelay`) SHALL be waited after the hooks and before cancelling components, so probes observe NOT_SERVING. `lifecycle.Stack` SHALL collect closers during wiring: `Push(f)` registers, `Close(ctx)` runs them once in reverse order and joins errors, so a composition root can release what it opened when a later step fails.

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

#### Scenario: Components and closers share one grace period
- **WHEN** components use part of the grace period and a closer would need more than what is left
- **THEN** `Run` returns within one `ShutdownGrace`, not two
- **VERIFIED BY**: `TestShutdownWorstCaseIsOneGrace`

#### Scenario: Stack closes in reverse order exactly once
- **WHEN** closers `a`, `b` are pushed, `b` fails, and `Close` is called twice
- **THEN** `b` then `a` run once each and the first `Close` returns `b`'s error
- **VERIFIED BY**: `TestStackClosesReverseOnce`

#### Scenario: Drain delay holds components after the hooks
- **WHEN** `DrainDelay` is 100ms and ctx is cancelled
- **THEN** components see cancellation no earlier than 100ms after the hooks ran
- **VERIFIED BY**: `TestDrainDelayHoldsComponents`

### Requirement: Postgres pool fails fast and is probed

`postgres.Open(ctx, cfg config.Postgres)` SHALL parse the URL, apply `MaxConns`, create the pool and ping it within 5s. It SHALL return an error if the ping fails. `postgres.Checker(pool)` SHALL return a health check that pings. `postgres.WithTx(ctx, pool, fn)` SHALL commit when `fn` returns nil and roll back, in a deferred step with a bounded timeout that ignores the cancellation of ctx, when it returns an error, panics or calls `runtime.Goexit`, without leaking the connection.

#### Scenario: Unreachable database fails at open
- **WHEN** `Open` targets a closed port
- **THEN** it returns an error before any query runs
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/postgres` `TestOpenFailsOnUnreachableDB`

#### Scenario: WithTx rolls back on error
- **WHEN** `fn` inserts a row and returns an error
- **THEN** the row is not visible afterwards
- **VERIFIED BY**: `TestWithTxRollsBackOnError` (testkit Postgres)

#### Scenario: WithTx rolls back on panic
- **WHEN** `fn` panics inside a transaction, repeatedly
- **THEN** the panic propagates, no row is committed and the pool is not exhausted
- **VERIFIED BY**: `TestWithTxPanicDoesNotLeakConnection`

#### Scenario: WithTx rolls back when fn exits its goroutine
- **WHEN** `fn` calls `runtime.Goexit` inside a transaction on a one-connection pool
- **THEN** no row is committed and the connection can be acquired again
- **VERIFIED BY**: `TestWithTxGoexitRollsBack`

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

`resilience` SHALL expose `Retry(opts) (Policy, error)` and `Breaker(opts) (Policy, error)` built on failsafe-go behind an opaque `Policy` type, plus `Do(ctx, fn, policies...)`, which SHALL return an error without calling `fn` when a policy is the zero `Policy`. Zero-value options SHALL take defaults (3 attempts, 100ms to 5s backoff, breaker threshold 5, open delay 30s) and negative values SHALL be rejected with an error. Retries SHALL use exponential backoff with jitter. `Retryable(err)` SHALL be an allow-list: only gRPC `Unavailable`, `ResourceExhausted`, `Aborted` and `DeadlineExceeded` are retried; plain errors, `Internal` and context cancellation are not.

#### Scenario: Non-retryable codes are not retried
- **WHEN** `fn` returns `status.Error(codes.InvalidArgument, …)` under a 3-attempt retry
- **THEN** `fn` runs once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/resilience` `TestRetrySkipsNonRetryableCodes`

#### Scenario: Internal and plain errors are not retried
- **WHEN** `fn` returns `codes.Internal` or a plain error
- **THEN** `fn` runs once
- **VERIFIED BY**: `TestInternalAndPlainErrorsAreNotRetried`

#### Scenario: Zero options are bounded
- **WHEN** `Retry(RetryOptions{})` and `Breaker(BreakerOptions{})` are used
- **THEN** retries stop after 3 attempts and the breaker does not panic
- **VERIFIED BY**: `TestZeroValueRetryOptionsBounded` and `TestZeroValueBreakerOptionsDoNotPanic`

#### Scenario: Invalid options are rejected
- **WHEN** an option value is negative
- **THEN** the constructor returns an error
- **VERIFIED BY**: `TestInvalidOptionsRejected`

#### Scenario: Open breaker rejects calls
- **WHEN** failures exceed the breaker threshold
- **THEN** the next call returns the breaker-open error without running `fn`
- **VERIFIED BY**: `TestBreakerOpensAfterFailures`

#### Scenario: Closers keep a live context
- **WHEN** a component ignores cancellation for the whole grace period
- **THEN** it is abandoned after `ShutdownGrace` minus `CloseGrace` and the closers still receive about `CloseGrace` of time
- **VERIFIED BY**: `TestCloseGraceReservesTimeForClosers` and `TestDefaultCloseGraceKeepsClosersAlive`

#### Scenario: A zero Policy is an error, not a panic
- **WHEN** `Do` receives `resilience.Policy{}`
- **THEN** it returns an error and does not call `fn`
- **VERIFIED BY**: `TestDoRejectsZeroPolicy`

### Requirement: gRPC servers share one interceptor chain and safe errors

`grpcx.NewServer(opts)` SHALL return a `*grpc.Server` with:
- the otelgrpc stats handler;
- the unary and stream chain request-id → recovery → logging → auth (request-id outermost so panic logs carry it);
- the health service from `opts.Health`;
- reflection, always registered.

The request-id interceptor SHALL reuse an incoming `x-request-id` of at most 128 characters from `[A-Za-z0-9._-]`, generate one otherwise, store it in `reqctx` and send it back as a response header. `grpcx.Serve(srv, cfg config.GRPC)` SHALL return a lifecycle component that listens on `cfg.Addr()` and stops gracefully when its context ends, forcing a stop after `cfg.ShutdownGrace`. `grpcx.ServeListener(srv, lis, grace)` SHALL do the same on a caller-provided listener, for tests. `grpcx.Status(ctx, logger, err, table)` SHALL map errors per the ordered `ErrorTable` (see "Error rules are ordered and can fix the client message"). An error that already carries a gRPC status other than `Unknown` (for example from `auth.RequirePrincipal`) SHALL be returned unchanged. Any other error SHALL be logged and mapped to `codes.Internal` with the message `internal error`.

#### Scenario: A panicking handler returns Internal and the server keeps serving
- **WHEN** a handler panics
- **THEN** the caller receives `codes.Internal` and a following call succeeds
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestRecoveryKeepsServing`

#### Scenario: Request id round-trips
- **WHEN** a call sends `x-request-id=abc`
- **THEN** the handler sees `reqctx.RequestID(ctx)=="abc"` and the response header carries `abc`
- **VERIFIED BY**: `TestRequestIDPropagates`

#### Scenario: Oversized or malformed request ids are replaced
- **WHEN** a call sends an `x-request-id` longer than 128 characters or with a character outside `[A-Za-z0-9._-]`
- **THEN** the handler sees a generated id instead
- **VERIFIED BY**: `TestRequestIDRejectsOversizedOrBadCharset`

#### Scenario: Reflection is always on
- **WHEN** a server is built without any reflection setting
- **THEN** the reflection service is registered
- **VERIFIED BY**: `TestReflectionAlwaysRegistered`

#### Scenario: Unknown errors do not leak
- **WHEN** a handler returns `grpcx.Status(ctx, logger, errors.New("pq: password=secret"), table)`
- **THEN** the caller sees `Internal` with message `internal error`
- **VERIFIED BY**: `TestStatusHidesUnknownErrors`

#### Scenario: Existing gRPC status errors pass through
- **WHEN** a handler returns `grpcx.Status(ctx, logger, status.Error(codes.PermissionDenied, "missing scope"), table)`
- **THEN** the caller sees `PermissionDenied` with message `missing scope`, not `Internal`
- **VERIFIED BY**: `TestStatusPassesThroughGRPCStatus`

### Requirement: The example service is the reference wiring

`packages/go-sdk/examples/hello` SHALL wire `config`, `logx`, `otelx`, `postgres`, `healthx`, `auth`, `grpcx` and `lifecycle` in an `internal/app.Run(ctx context.Context, s config.Settings) error` composition root (`main` loads settings), with a short `cmd/hello/main.go` that only creates the signal context, loads settings and calls `app.Run`. A migration failure SHALL fail boot in every environment; only an unreachable database in `ENV=local` MAY be tolerated. SIGTERM SHALL drain health before the server stops.

#### Scenario: Health drains before stop
- **WHEN** the example runs and its context is cancelled
- **THEN** a health watcher observes NOT_SERVING before the listener closes
- **VERIFIED BY**: `platform-core/packages/go-sdk/examples/hello/internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: Calls without a principal are rejected
- **WHEN** `Greet` is called without `x-principal-*` metadata
- **THEN** the caller receives `Unauthenticated`
- **VERIFIED BY**: `TestGreetRejectsMissingPrincipal`

#### Scenario: An unreachable database fails boot outside local
- **WHEN** the example boots with an unreachable database and `ENV=prod`
- **THEN** `app.Run` returns an error
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachableOutsideLocal`

### Requirement: Kafka producer propagates trace and request ids

`kafkax.NewProducer(c config.Kafka)` SHALL return a franz-go producer that writes synchronously with OTel instrumentation. `Publish(ctx, topic, key, value)` SHALL add record headers `traceparent` (from the active span) and `x-request-id` (from `reqctx`) when present. It SHALL return the produce error.

#### Scenario: Headers carry trace and request ids
- **WHEN** `Publish` runs with a context holding a span and request id `r1`
- **THEN** the produced record has a `traceparent` header for that span and an `x-request-id` header `r1`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestPublishAddsTraceAndRequestHeaders` (testkit Redpanda)

#### Scenario: Produce failure is returned, not swallowed
- **WHEN** the broker is unreachable and the record delivery timeout elapses
- **THEN** `Publish` returns a non-nil error
- **VERIFIED BY**: `TestPublishReturnsErrorWhenBrokerDown`

### Requirement: Outbox relays rows with retry and park

`outbox.NewStore(pool, table)` SHALL operate on an existing table with the shared column set. `EnqueueTx(ctx, tx, Row)` SHALL insert inside the caller's transaction. `outbox.Relayer` SHALL be a lifecycle component. On each poll it SHALL:
- claim up to `BatchSize` pending, available, unlocked rows with `FOR UPDATE SKIP LOCKED`, locking them for `ClaimLock`;
- publish each row by key `aggregate_id`;
- on success, mark the row published;
- on failure, increment `attempts`, store the error and set `available_at` to now plus `min(base·2^(attempts-1), max)`;
- once `attempts` reaches `MaxAttempts`, set status `failed` (parked).

#### Scenario: A published row is not relayed twice
- **WHEN** a row is enqueued and two relayers poll concurrently
- **THEN** it is published exactly once and marked published
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/outbox` `TestConcurrentRelayersPublishOnce`

#### Scenario: A failing row backs off and is parked
- **WHEN** publishing a row fails `MaxAttempts` times
- **THEN** its `available_at` grows between attempts and its status ends as `failed`, while other rows still publish
- **VERIFIED BY**: `TestFailingRowBacksOffThenParks`

#### Scenario: Enqueue rolls back with the business transaction
- **WHEN** `EnqueueTx` runs in a transaction that is rolled back
- **THEN** no outbox row exists
- **VERIFIED BY**: `TestEnqueueRollsBackWithTx`

### Requirement: Consumer never commits past an unhandled record

`consume.Consumer` SHALL be a lifecycle component reading a consumer group with manual commits. For each record it SHALL call the handler. A handler error SHALL be retried with exponential backoff up to `MaxAttempts`, unless it is wrapped with `consume.Permanent`. A record whose handling finally fails SHALL be produced to `DLQTopic`, carrying the original key, value and headers plus `error` and `source-topic` headers, before its offset is committed. If the DLQ produce fails, `Run` SHALL return an error without committing that record.

#### Scenario: Poison record goes to the DLQ and the offset moves on
- **WHEN** the handler returns `consume.Permanent(err)` for one record
- **THEN** the record is on the DLQ topic with an `error` header, it is not retried, and the next record is handled
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestPermanentErrorGoesToDLQ` (testkit Redpanda)

#### Scenario: DLQ failure stops without committing
- **WHEN** handling fails and the DLQ produce also fails
- **THEN** `Run` returns an error and, after restart, the same record is delivered again
- **VERIFIED BY**: `TestDLQFailureDoesNotCommit`

### Requirement: Dedupe is transactional and per consumer

`consume.Dedupe.Seen(ctx, tx, consumer, eventID)` SHALL insert `(consumer, event_id)` into `processed_events` with `ON CONFLICT DO NOTHING` inside `tx`, and report whether the pair was already present. It SHALL return database errors instead of treating them as not processed.

#### Scenario: Same event for two consumers is processed by both
- **WHEN** consumers `a` and `b` each see event `e1` once
- **THEN** both get `seen=false`, and a second call for `a` gets `seen=true`
- **VERIFIED BY**: `TestDedupeIsPerConsumer`

#### Scenario: Rolled-back handling is not marked processed
- **WHEN** `Seen` runs in a transaction that is rolled back
- **THEN** a later `Seen` for the same pair returns `seen=false`
- **VERIFIED BY**: `TestDedupeRollsBackWithTx`

### Requirement: Dependents stop only after the primary component

`lifecycle.StopAfter(primary, dependents...)` SHALL return a component that runs all of them. On cancellation it SHALL first cancel `primary` and wait for it to return, then cancel the dependents and wait for them.

#### Scenario: Worker outlives the server during shutdown
- **WHEN** a `StopAfter(server, worker)` component's context is cancelled
- **THEN** `worker` observes cancellation only after `server` has returned, never before
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/lifecycle` `TestStopAfterOrdersShutdown`

### Requirement: Upstream clients propagate identity and ids

`upstream.Dial(addr, opts...)` SHALL create a gRPC client with otelgrpc. It SHALL forward the incoming `x-request-id`, and either forward the incoming principal metadata or, with `upstream.AsService(id, scopes...)`, replace it with that service principal. `upstream.Bounded(ctx, d)` SHALL return a context whose deadline is the earlier of the parent deadline minus a margin and now plus `d`.

#### Scenario: Service principal replaces the caller and the buyer is not forwarded
- **WHEN** a call is made with `AsService("team-order", "listing.read")` while the incoming principal is a buyer
- **THEN** the upstream sees `x-principal-id=team-order`, type `service`, scope `listing.read`, and not the buyer id
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/upstream` `TestAsServiceReplacesPrincipal`

### Requirement: Feature flags fail open to defaults

`flags.New(ctx, config.Flags)` SHALL evaluate booleans through OpenFeature with the Flipt provider when `FEATURE_FLAGS_ENABLED=true`, bounded by `FEATURE_FLAGS_EVAL_TIMEOUT_MS`. When flags are disabled or Flipt is unreachable, it SHALL return the caller's default.

#### Scenario: Unreachable Flipt returns the default
- **WHEN** Flipt is unreachable and `BooleanEnabled(ctx, "checkout-enabled", true)` is called
- **THEN** it returns `true` within the eval timeout
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/flags` `TestUnreachableFliptReturnsDefault`

### Requirement: Outbox enqueue is idempotent on event id

`outbox.Store.EnqueueTx` SHALL insert with `ON CONFLICT (event_id) DO NOTHING`, so enqueuing an existing event id inside a transaction neither fails nor duplicates.

#### Scenario: Duplicate enqueue does not abort the transaction
- **WHEN** the same event id is enqueued twice in one transaction, which then commits
- **THEN** the commit succeeds and exactly one outbox row exists
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/outbox` `TestEnqueueDuplicateIsNoop`

### Requirement: Upstream calls can use a per-call service principal

`upstream.WithServicePrincipal(ctx, id, scopes...)` SHALL return a context. A call made with that context SHALL send that service principal instead of the forwarded or connection-level principal.

#### Scenario: Per-call service principal overrides forwarding, not other calls
- **WHEN** two calls on one connection are made, one with `WithServicePrincipal(ctx, "team-order", "listing.read")` and one without, while the incoming principal is a buyer
- **THEN** the first sends `team-order`/`service`/`listing.read`, and the second forwards the buyer, not the service principal
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/upstream` `TestPerCallServicePrincipal`

### Requirement: Flags can be built over any provider

`flags.NewWithProvider(p openfeature.FeatureProvider, timeout time.Duration)` SHALL return a client evaluating through `p`. Evaluation errors SHALL fall back to the caller's default.

#### Scenario: In-memory provider toggles a flag and an unknown flag does not fail
- **WHEN** a client over an in-memory provider with `checkout-enabled=false` evaluates it with default `true`
- **THEN** it returns `false`, and an unknown flag returns its default rather than failing
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/flags` `TestNewWithProviderEvaluates`

### Requirement: Consumer survives fetch errors

`consume.Consumer.Run` SHALL log fetch errors and keep polling. It SHALL NOT return on them.

#### Scenario: Transient fetch error does not stop the loop
- **WHEN** a fetch returns an error and later records arrive
- **THEN** the later records are handled and `Run` has not returned
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestFetchErrorDoesNotStopLoop`

### Requirement: Consumer clients accept extra options

`kafkax.NewConsumerClient(c, group, topics, opts...)` SHALL accept extra `kgo.Opt` values, applied after the defaults.

#### Scenario: Auto topic creation can be enabled for a consumer
- **WHEN** a consumer client is built with `kgo.AllowAutoTopicCreation()` for a topic that does not exist yet
- **THEN** it consumes records produced later, without failing on the unknown topic
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestConsumerClientAcceptsOptions`

### Requirement: Scope checks can admit scoped anonymous callers

`auth.RequireScopesAllowAnonymous(ctx, scopes...)` SHALL return `codes.Unauthenticated` when no principal is present, and `codes.PermissionDenied` when any listed scope is missing. It SHALL return the principal otherwise, including an anonymous principal that holds every listed scope.

#### Scenario: Anonymous caller with the public scope is admitted
- **WHEN** the anonymous principal holds `listing.read` and `RequireScopesAllowAnonymous(ctx, "listing.read")` is called
- **THEN** it returns the principal and no error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auth` `TestRequireScopesAllowAnonymousAdmitsScopedAnonymous`

#### Scenario: Anonymous caller without the scope is denied, not unauthenticated
- **WHEN** the anonymous principal lacks `listing.write`
- **THEN** it returns `PermissionDenied`
- **VERIFIED BY**: `TestRequireScopesAllowAnonymousDeniesMissingScope`

### Requirement: The Go service template follows ADR-0016

`platform-core/templates/_service-go` SHALL mirror `examples/hello`: the layout, `internal/app` wiring, a `Makefile` that includes `../platform-core/make/go-service.mk`, `.env.example`, and a README with the rename and `go get` steps. Its `go.mod` SHALL `require` the SDK at a tag, without a `replace` and without a `third_party` directory. It SHALL NOT contain its own interceptors, platform code, `.golangci.yml` or copy of `go-service.mk`.

#### Scenario: A service scaffolded from the template builds without platform copies
- **WHEN** the template is copied to a scratch dir, renamed, and its SDK `require` resolved to the current tag
- **THEN** `go build ./...` and `go vet ./...` pass and no `internal/interceptor`, `internal/grpcserver` or `third_party` exists
- **VERIFIED BY**: `platform-core/tools/template_smoke_test.sh`

### Requirement: Principal type gates

`auth` SHALL export `TypeUser = "user"`, `TypeService = "service"`, `TypeAnonymous = "anonymous"`, `RequireService(ctx, scopes...) (Principal, error)` and `RequireUser(ctx) (Principal, error)`. A missing or anonymous principal SHALL yield Unauthenticated; a principal of the wrong type or missing a scope SHALL yield PermissionDenied.

#### Scenario: RequireService rejects a user principal
- **WHEN** a user principal holding every requested scope calls `RequireService(ctx, "s")`
- **THEN** it returns PermissionDenied
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auth` `TestRequireServiceRejectsUser`

#### Scenario: RequireUser does not admit service or anonymous callers
- **WHEN** `RequireUser` runs for a service principal and for the anonymous principal
- **THEN** the service gets PermissionDenied and the anonymous caller gets Unauthenticated
- **VERIFIED BY**: `TestRequireUserRejectsServiceAndAnonymous`

### Requirement: Token wire types are shared

New package `jwtx` SHALL export `Claims` (registered claims plus `name`, `typ`, `scopes`, `sid` JSON fields) and `JWKS{Keys []JWK}` with RFC 7517 field names, so issuer and verifier share one contract.

#### Scenario: Claims JSON field names do not drift
- **WHEN** a `Claims` value is marshalled and unmarshalled
- **THEN** the JSON object has exactly the keys `name`, `typ`, `scopes`, `sid` besides the registered ones, and the round trip is lossless
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/jwtx` `TestClaimsJSONContract`

### Requirement: Idempotency keys follow one rule

New package `idempotency` SHALL export `MDKey = "idempotency-key"` and `Validate(key string) error`, accepting 1 to 255 printable ASCII characters.

#### Scenario: Out-of-rule keys are not accepted
- **WHEN** `Validate` gets an empty key, a 256-character key, or a key with a control character
- **THEN** it returns an error for each, and returns nil for a 255-character printable key
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/idempotency` `TestValidateRule`

### Requirement: Error rules are ordered and can fix the client message

`grpcx.ErrorTable` SHALL be an ordered list built with `grpcx.NewErrorTable(rules ...grpcx.Rule) (ErrorTable, error)`, which SHALL return an error for a rule with a nil `Target` or with `codes.OK`, where `Rule{Target error; Code codes.Code; Message string}`. `grpcx.Status` SHALL use the first rule whose Target matches with `errors.Is`, so the order is the precedence. A matching rule with a Message SHALL use it as the status message; an empty Message SHALL send `Target.Error()`, never the wrapped chain. `context.Canceled` and `context.DeadlineExceeded` SHALL map to `Canceled` and `DeadlineExceeded` and be logged at Info. Unmatched errors keep becoming Internal with the fixed internal message.

#### Scenario: Rule message replaces wrapped error text
- **WHEN** `fmt.Errorf("listing 7: %w", ErrNotFound)` matches `Rule{Target: ErrNotFound, Code: NotFound, Message: "not_found"}`
- **THEN** the status is NotFound with message `not_found`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusUsesRuleMessage`

#### Scenario: A rule without message sends the sentinel text, not the chain
- **WHEN** the same error matches `Rule{Target: ErrNotFound, Code: NotFound}`
- **THEN** the message is the sentinel's own text, not `listing 7: ...`
- **VERIFIED BY**: `TestStatusDefaultsToSentinelText`

#### Scenario: The first matching rule wins
- **WHEN** an error matches two rules of one table
- **THEN** the status carries the code of the rule listed first, on every call
- **VERIFIED BY**: `TestStatusFirstMatchingRuleWins`

#### Scenario: Context errors are not internal errors
- **WHEN** the error is `context.Canceled` or wraps `context.DeadlineExceeded`
- **THEN** the status is Canceled or DeadlineExceeded and nothing is logged at error level
- **VERIFIED BY**: `TestStatusMapsContextErrorsWithoutErrorLog`

#### Scenario: Bad rules are rejected
- **WHEN** `NewErrorTable` receives a rule with a nil `Target` or with `codes.OK`
- **THEN** it returns an error
- **VERIFIED BY**: `TestNewErrorTableRejectsBadRules`

### Requirement: Local-only database fallback

`postgres.OpenOrLocal(ctx, c config.Postgres, rt config.Runtime, logger) (*pgxpool.Pool, error)` SHALL return the pool when the database is reachable; when it is not, SHALL return `(nil, nil)` and log a warning only if `rt.IsLocal()` (`ENV` is `local` or `test`), and return the error otherwise. A nil logger SHALL default to `slog.Default()`.

#### Scenario: Unreachable database outside local does not fall back
- **WHEN** the database is unreachable with `ENV=dev` and with `ENV=local`
- **THEN** `dev` returns an error and `local` returns a nil pool and nil error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/postgres` `TestOpenOrLocalFallsBackOnlyLocally`

#### Scenario: A nil logger does not panic
- **WHEN** `OpenOrLocal` runs with a nil logger against an unreachable database with `ENV=local`
- **THEN** it returns a nil pool and nil error
- **VERIFIED BY**: `TestOpenOrLocalNilLogger`

### Requirement: Config helpers for strict envs, durations and empty values

`config.Runtime.IsStrict()` SHALL be true for `staging`, `stage`, `prod`, `production` (case-insensitive). `config.PositiveDuration(key, raw string, def time.Duration) (time.Duration, error)` SHALL return def for an empty raw value and an error naming key for an unparsable or non-positive value. A field tagged `keepempty:"true"` SHALL stay empty when its variable is set to the empty string, instead of taking its default.

#### Scenario: Strict envs are recognised and dev is not
- **WHEN** `IsStrict` runs for `PROD`, `stage`, `dev` and `local`
- **THEN** it is true, true, false, false
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestIsStrict`

#### Scenario: Non-positive durations are not accepted
- **WHEN** `PositiveDuration("X", "0s", time.Second)` and `PositiveDuration("X", "", time.Second)` run
- **THEN** the first returns an error naming `X` and the second returns one second
- **VERIFIED BY**: `TestPositiveDuration`

#### Scenario: Explicit empty value is kept
- **WHEN** a `keepempty` field with default `a` is loaded with its variable set to `""`
- **THEN** the field is empty, and with the variable unset it is `a`
- **VERIFIED BY**: `TestKeepEmpty`

### Requirement: Offset pagination helpers

New package `pagination` SHALL export `Clamp(size, def, max int) int` (non-positive → def, above max → max), `EncodeOffset(n int) string` and `DecodeOffset(tok string) (int, error)`; an empty token decodes to 0 and a malformed or negative token returns an error.

#### Scenario: Malformed tokens are not decoded
- **WHEN** `DecodeOffset` gets `EncodeOffset(40)`, `""` and `"zz"`
- **THEN** it returns 40, 0 and an error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/pagination` `TestOffsetToken`

### Requirement: Ticker and HTTP components

`lifecycle.Every(interval time.Duration, fn func(context.Context) error, opts ...lifecycle.EveryOption) (Component, error)` SHALL return an error for a non-positive interval. The component SHALL run fn on each tick (first call after one interval, or immediately with `lifecycle.RunAtStart()`), log a returned error through `lifecycle.WithLogger(l)` (default `slog.Default()`) and keep running, and stop when ctx ends. `lifecycle.HTTP(srv *http.Server, grace time.Duration) Component` SHALL serve until ctx ends and then call `Shutdown` bounded by grace.

#### Scenario: A failing tick does not stop the worker
- **WHEN** fn fails on its first call
- **THEN** it is called again on the next tick, and Run returns nil after ctx is cancelled
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/lifecycle` `TestEveryKeepsRunningAfterError`

#### Scenario: The first call waits one interval unless RunAtStart is set
- **WHEN** `Every` runs without `RunAtStart`
- **THEN** fn is not called before the first interval has passed
- **VERIFIED BY**: `TestEveryWaitsForFirstTickWithoutRunAtStart`

#### Scenario: A non-positive interval is rejected
- **WHEN** `Every` gets an interval of zero or less
- **THEN** it returns an error instead of a component
- **VERIFIED BY**: `TestEveryRejectsNonPositiveInterval`

#### Scenario: HTTP server drains in-flight requests
- **WHEN** ctx is cancelled while a request is in flight
- **THEN** the request completes and Run returns nil within grace
- **VERIFIED BY**: `TestHTTPShutsDownGracefully`

### Requirement: Flags degrade on setup failure and evaluate with a context

`flags.NewOrDisabled(ctx, c, logger) (Evaluator, func(context.Context) error)` SHALL return the Flipt-backed evaluator, or `Disabled()` with a warning when setup fails; the closer is always safe to call. `flags.NewWithProvider(ctx, p, timeout, logger) (*Client, error)` SHALL take the same leading arguments as `flags.New`. `Evaluator.BooleanEnabled(ctx, flag, def, opts ...EvalOption) bool` SHALL use the id of the principal in ctx as the targeting key (`anonymous` without one) unless `WithTargetingKey` overrides it, SHALL add `WithAttributes` to the evaluation context, and SHALL return def on any error or timeout. `Close` SHALL shut down only the client's own provider, not other clients.

#### Scenario: Setup failure does not stop the service
- **WHEN** the provider cannot be set up because `FLIPT_ADDR` is empty
- **THEN** the evaluator returns caller defaults and the closer returns nil
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/flags` `TestNewOrDisabledDegrades`

#### Scenario: The targeting key comes from the principal
- **WHEN** a flag is evaluated with a principal in ctx and again without one
- **THEN** the evaluation context key is the principal id, then `anonymous`
- **VERIFIED BY**: `TestTargetingKeyFromPrincipal`

#### Scenario: Options override the key and add attributes
- **WHEN** `WithTargetingKey("k")` and `WithAttributes` are passed
- **THEN** the evaluation context carries key `k` and the attributes
- **VERIFIED BY**: `TestEvalOptionsOverrideKeyAndAddAttributes`

#### Scenario: Closing one client keeps another working
- **WHEN** one of two clients is closed
- **THEN** the other still evaluates
- **VERIFIED BY**: `TestCloseKeepsOtherClientWorking`

### Requirement: Batch consumer

`consume.NewBatchConsumer(client, h BatchHandler, dlq DLQProducer, o BatchOptions) (*BatchConsumer, error)` with `BatchHandler func(ctx, []Record) error` SHALL hand each fetched batch to h; on nil it SHALL commit; on an error wrapped with `Permanent` it SHALL send every record of the batch to the DLQ with the same headers as the per-record consumer and then commit; on any other error it SHALL retry the same batch after backoff and SHALL NOT commit, up to `BatchOptions.MaxAttempts` (default 5), after which the whole batch goes to the DLQ.

#### Scenario: Transient batch failure is retried without commit
- **WHEN** h fails transiently once and then succeeds
- **THEN** h sees the same records twice and offsets are committed only after the success
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestBatchRetriesWithoutCommit`

#### Scenario: Permanent batch failure goes to the DLQ
- **WHEN** h returns `Permanent(err)`
- **THEN** every record is on the DLQ with `error` and `source-topic` headers and the batch is committed
- **VERIFIED BY**: `TestBatchPermanentGoesToDLQ`

#### Scenario: A poison batch is dead-lettered after the attempt limit
- **WHEN** h keeps failing with a non-Permanent error
- **THEN** after `MaxAttempts` tries the batch is on the DLQ and committed
- **VERIFIED BY**: `TestPoisonBatchGoesToDLQAfterMaxAttempts`

#### Scenario: The attempt limit has a default
- **WHEN** `BatchOptions.MaxAttempts` is zero
- **THEN** the consumer uses 5 attempts
- **VERIFIED BY**: `TestBatchDefaultMaxAttempts`

### Requirement: Dedupe can be checked without writing

`consume.NewDedupe(table) (Dedupe, error)` SHALL validate the table name, and `consume.DedupeSchemaSQL(table) (string, error)` SHALL validate it too. `consume.Dedupe.Processed(ctx, q, consumer, eventID)` SHALL report whether `(consumer, eventID)` exists, without inserting, and SHALL return database errors rather than false. `Seen` SHALL accept any `Executor` (a pool, connection or transaction).

#### Scenario: Processed reports existing pairs only
- **WHEN** `(a, e1)` was marked and `(b, e1)` was not
- **THEN** `Processed(a, e1)` is true, `Processed(b, e1)` is false, and no row is inserted by either call
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestDedupeProcessedIsReadOnly`

#### Scenario: Database errors are not reported as not processed
- **WHEN** the dedupe table does not exist
- **THEN** `Processed` returns a non-nil error
- **VERIFIED BY**: `TestDedupeProcessedReturnsErrors`

#### Scenario: An invalid table name is rejected at construction
- **WHEN** `NewDedupe` gets a name that is not a plain identifier
- **THEN** it returns an error
- **VERIFIED BY**: `TestNewDedupeValidatesTable`

### Requirement: Consumer clients can read without a group

`kafkax.NewConsumerClient` with an empty group SHALL return a client that consumes the topics from the start, with no consumer group and no commits.

#### Scenario: Group-less client reads existing records and does not commit
- **WHEN** records exist on a topic and a group-less client is built for it
- **THEN** it reads them from the start, and no consumer group appears on the broker
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestGroupLessConsumerClient`

### Requirement: Upstream client options and retry predicates

`upstream.NewClient(addr, opts ...Option) (*grpc.ClientConn, error)` SHALL return a lazily connecting client that balances with `round_robin` by default. `upstream.WithDialOptions(opts ...grpc.DialOption) Option` SHALL append dial options after the SDK defaults, so they can override them. `resilience.RetryOptions.RetryIf func(error) bool` SHALL decide retries when set; nil keeps `Retryable`.

#### Scenario: Extra dial options are applied
- **WHEN** `NewClient` gets `WithDialOptions` carrying a unary interceptor
- **THEN** the interceptor sees each call
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/upstream` `TestDialOptionsApplied`

#### Scenario: Calls are spread across resolved addresses
- **WHEN** the resolver returns two addresses and the client makes several calls
- **THEN** both backends receive calls
- **VERIFIED BY**: `TestDefaultsToRoundRobin`

#### Scenario: RetryIf narrows retries
- **WHEN** `RetryIf` admits only Unavailable and fn returns DeadlineExceeded under 3 attempts
- **THEN** fn runs once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/resilience` `TestRetryIfOverridesDefault`

### Requirement: Shared test fixtures

`testkit` SHALL export `FreePort(t) int`, `Eventually(t, timeout, fn func() bool)`, `Recorder` (its `PublishRecord` satisfies `outbox.Publisher` and `consume.DLQProducer`; records topic/key/value/headers; `FailOn(key)`), `SharedPostgres(t) string` (one container per test binary) with `Schema(t, dsn) string` (a DSN whose `search_path` is a fresh schema dropped at cleanup), and `Produce(t, brokers, topic, key, value)` / `ReadAll(t, brokers, topic, n, timeout) []*kgo.Record`. With `GO_SDK_REQUIRE_DOCKER` set and no Docker, every `SharedPostgres` caller SHALL fail, not only the first.

#### Scenario: Schemas isolate tests that share a container
- **WHEN** two tests create the same table through `Schema` DSNs on one `SharedPostgres`
- **THEN** neither sees the other's rows, and the schemas are gone after cleanup
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/testkit` `TestSchemaIsolation`

#### Scenario: Recorder can fail one key and not others
- **WHEN** `FailOn("k1")` is set and keys k1 and k2 are published
- **THEN** k1 returns an error and only k2 is recorded
- **VERIFIED BY**: `TestRecorderFailOn`

#### Scenario: Every caller fails when Docker is required and missing
- **WHEN** `GO_SDK_REQUIRE_DOCKER` is set and Docker is unavailable
- **THEN** each `SharedPostgres` call fails the test
- **VERIFIED BY**: `TestSharedPostgresFailsEveryCallerWhenDockerRequired`

### Requirement: Outbox rows publish in order with headers

`outbox.Publisher` SHALL be `PublishRecord(ctx, *kgo.Record) error`. The relayer SHALL publish every row as a record keyed by `AggregateID`, on the row's `Topic` or else the relayer's `Topic`, with the `kafkax` headers `event-id`, `event-type`, `x-request-id` and the `traceparent` stored at enqueue. Order SHALL hold per aggregate: a row SHALL NOT be claimed while an older unpublished row of the same aggregate exists, whether it is backing off, leased or parked; other aggregates SHALL NOT be held back. One relay batch SHALL be bounded by half of `Outbox.ClaimLock` (`OUTBOX_CLAIM_LOCK`). `EnqueueTx` SHALL reject an empty `EventID` with `ErrEmptyEventID`, and `Store.Purge(ctx, olderThan)` SHALL delete only published rows older than the duration. Only record-level errors (`MessageTooLarge`, `RecordListTooLarge`, `CorruptMessage`, `InvalidRecord`, `InvalidTimestamp`, or an error wrapped with `outbox.Permanent`) SHALL park a row; every other error SHALL retry with capped backoff, logging at ERROR once attempts exceed `RelayerOptions.AlertAfter`. `Store.Requeue` and `Store.RequeueParked` SHALL return parked rows to pending with attempts reset. The head scan SHALL resume after the last claimed aggregate and wrap around, so every aggregate is served. `SchemaSQL` SHALL create a fresh table; `UpgradeSQL` SHALL return the steps that move a table created by an older SDK to the new schema while numbering existing rows in `(created_at, event_id)` order.

#### Scenario: Rows of one aggregate publish in enqueue order
- **WHEN** several rows of one aggregate are enqueued, some in the same transaction
- **THEN** they are published in enqueue order
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/outbox` `TestOrderPerAggregate`

#### Scenario: A failing row blocks its own aggregate only
- **WHEN** an older row of aggregate A fails and rows of A and B are pending
- **THEN** B's rows are published and A's newer rows wait
- **VERIFIED BY**: `TestFailingRowBlocksItsAggregateOnly`

#### Scenario: Topic-level errors do not park rows
- **WHEN** publishing fails with `UnknownTopicOrPartition` or a topic authorization error
- **THEN** the row backs off and stays pending, and a `MessageTooLarge` error parks it
- **VERIFIED BY**: `TestPublishErrorClassification`

#### Scenario: Upgrading an older table keeps creation order
- **WHEN** an older table with rows still unnumbered at Switch is upgraded and new rows are inserted after Switch
- **THEN** every row has a unique `seq` in creation order and the relayer publishes in that order
- **VERIFIED BY**: `TestUpgradeSQLKeepsOrderOfAV0Table`

#### Scenario: High aggregate keys are not starved
- **WHEN** more aggregates have a backlog than the batch size
- **THEN** every aggregate publishes a row within a bounded number of passes
- **VERIFIED BY**: `TestHeadScanCursorServesEveryAggregate`

#### Scenario: A parked row keeps blocking its aggregate
- **WHEN** a row of aggregate A is parked as failed
- **THEN** newer rows of A are not claimed
- **VERIFIED BY**: `TestParkedRowBlocksItsAggregate`

#### Scenario: Published records carry the headers
- **WHEN** a row is enqueued in a context with a request id and a trace
- **THEN** the published record has `event-id`, `event-type`, `x-request-id` and `traceparent` headers
- **VERIFIED BY**: `TestPublishedRecordCarriesHeaders`

#### Scenario: A row topic overrides the relayer topic
- **WHEN** one row names a topic and another does not
- **THEN** the first goes to its topic and the second to the relayer's
- **VERIFIED BY**: `TestRowTopicOverridesRelayerTopic`

#### Scenario: A blocked broker cannot outlive the claim lock
- **WHEN** the publisher blocks
- **THEN** `RelayOnce` returns before the claim lock expires and the rows are released
- **VERIFIED BY**: `TestRelayBatchBoundedUnderBlockingPublisher`

#### Scenario: Empty event ids are rejected
- **WHEN** `EnqueueTx` gets a row with an empty `EventID`
- **THEN** it returns `ErrEmptyEventID` and inserts nothing
- **VERIFIED BY**: `TestEnqueueRejectsEmptyEventID`

#### Scenario: Purge removes old published rows only
- **WHEN** `Purge` runs over published, pending and recent rows
- **THEN** only published rows older than the duration are deleted
- **VERIFIED BY**: `TestPurgeDeletesOnlyOldPublishedRows`

#### Scenario: The schema upgrades an old table
- **WHEN** `SchemaSQL` is applied to a table created by v0.2.1
- **THEN** the new columns and indexes exist and old rows keep working
- **VERIFIED BY**: `TestSchemaSQLUpgradesTableFromOldSchema`

### Requirement: Consumers pass context, commit once per fetch and validate their input

`consume.NewConsumer(client, h, dlq, o) (*Consumer, error)` and `NewBatchConsumer` SHALL return an error for a nil client, handler or DLQ producer and for an empty `DLQTopic`. The handler context SHALL carry the W3C trace context and `x-request-id` of the record; a batch handler context SHALL carry a span linked to each record's span. Offsets SHALL be committed once per fetch, after every record of it is handled or dead-lettered. When handling or dead-lettering a record fails, the records before it in the fetch SHALL still be committed and the failed record and those after it SHALL NOT, so a restart redelivers from the failed record. `consume.Record` SHALL carry `Partition`, `Timestamp` and `Headers []Header`, which keeps duplicate header keys.

#### Scenario: Trace and request id reach the handler
- **WHEN** a record carries `traceparent` and `x-request-id` headers
- **THEN** the handler's context has that trace and that request id
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestTraceAndRequestIDReachHandlerContext`

#### Scenario: A batch span links to its records
- **WHEN** a batch of traced records is handled
- **THEN** the batch span links to each record's span
- **VERIFIED BY**: `TestBatchContextLinksRecordSpans`

#### Scenario: Records expose partition, timestamp and duplicate headers
- **WHEN** a record has two headers with the same key
- **THEN** `Record.Headers` holds both, and partition and timestamp are set
- **VERIFIED BY**: `TestRecordCarriesPartitionTimestampAndDuplicateHeaders`

#### Scenario: A revoked partition is not rewound or committed twice
- **WHEN** a partition is revoked in the middle of a fetch
- **THEN** its offsets are neither rewound nor committed twice
- **VERIFIED BY**: `TestRevokeMidFetchDoesNotRewindOrDoubleCommit`

#### Scenario: A failed record is never committed and rebalances stay possible
- **WHEN** a record fails and its dead-lettering also fails
- **THEN** nothing is committed and `AllowRebalance` is still called
- **VERIFIED BY**: `TestFailedRecordIsNeverCommittedAndRebalanceIsAllowed`

#### Scenario: A failed DLQ publish commits nothing
- **WHEN** the DLQ producer returns an error
- **THEN** the fetch is not committed
- **VERIFIED BY**: `TestDLQFailureCommitsNothing`

#### Scenario: Constructors reject invalid arguments
- **WHEN** a constructor gets a nil handler, a nil DLQ producer or an empty `DLQTopic`
- **THEN** it returns an error
- **VERIFIED BY**: `TestConstructorsRejectInvalidArguments`

### Requirement: Kafka clients are bounded and rebalance-safe

`kafkax.NewProducer` SHALL set a default `RecordDeliveryTimeout` of 30s that caller options can override. A group client from `kafkax.NewConsumerClient` SHALL use `kgo.BlockRebalanceOnPoll`; callers that poll it directly SHALL call `AllowRebalance` after committing each fetch and before `Close`. `kafkax.Checker(c)` SHALL return a health check that pings the brokers.

#### Scenario: The producer has a delivery timeout
- **WHEN** a producer is built without options
- **THEN** its record delivery timeout is 30s
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestProducerDefaultsRecordDeliveryTimeout`

#### Scenario: Group clients block rebalance on poll
- **WHEN** a group client is built
- **THEN** a rebalance cannot complete until `AllowRebalance` is called
- **VERIFIED BY**: `TestGroupConsumerClientBlocksRebalanceOnPoll`

#### Scenario: The health check reports the ping result
- **WHEN** the pinger succeeds and then fails
- **THEN** the checker returns nil, then that error
- **VERIFIED BY**: `TestCheckerReportsPingResult`

### Requirement: Principal types are typed

`auth.Principal.Type` SHALL be `auth.PrincipalType`, with constants `TypeUser`, `TypeService` and `TypeAnonymous`. `idempotency.Validate` SHALL return an error that matches `idempotency.ErrInvalidKey` with `errors.Is`.

#### Scenario: Constants keep their wire values
- **WHEN** the constants are converted to strings
- **THEN** they are `user`, `service` and `anonymous`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auth` `TestPrincipalTypeConstants`

#### Scenario: An invalid key matches ErrInvalidKey
- **WHEN** `Validate` rejects a key
- **THEN** `errors.Is(err, ErrInvalidKey)` is true
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/idempotency` `TestValidateReturnsErrInvalidKey`

### Requirement: The SDK is a tagged Go module that services require

The SDK module path SHALL be `github.com/buidangphuc/agora/platform-core/packages/go-sdk`, and every import inside the SDK, its examples and the service template SHALL use it. A release SHALL be a git tag `platform-core/packages/go-sdk/vX.Y.Z`. A service SHALL obtain the SDK with a `require` on a tag and SHALL NOT use a `replace` directive or a `third_party/go-sdk` directory for it. The SDK `README.md` and `CHANGELOG.md` SHALL name the release `v1.0.1` for this change. The repository SHALL NOT contain `platform-core/tools/sync-go-sdk.sh`, `platform-core/tools/check-go-sdk-vendor.sh` or their tests.

#### Scenario: Module path and imports use the fetchable path
- **WHEN** the SDK `go.mod` and every `.go` file under `platform-core/packages/go-sdk` are read
- **THEN** the module line is `github.com/buidangphuc/agora/platform-core/packages/go-sdk` and no file imports `github.com/buidangphuc/platform-core/`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestModulePathIsFetchable`

#### Scenario: Changelog names the release
- **WHEN** the first release heading of `CHANGELOG.md` and the version line of `README.md` are read
- **THEN** both are `v1.0.1`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestReleaseVersionIsDocumented`

#### Scenario: The service template requires a tag and does not vendor
- **WHEN** `platform-core/templates/_service-go/go.mod` is read
- **THEN** it has a `require` for the go-sdk module, does not contain a `replace` for it, and the template has no `third_party` directory
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestTemplateRequiresTagWithoutReplace`

#### Scenario: Vendoring tooling is gone
- **WHEN** the files `platform-core/tools/sync-go-sdk.sh`, `sync-go-sdk_test.sh`, `check-go-sdk-vendor.sh` and `check-go-sdk-vendor_test.sh` are looked up
- **THEN** none exists and no workflow or `ci_local.sh` references them
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestVendoringToolingIsRemoved`

### Requirement: Services share the make and lint files by reference

`platform-core/make/go-service.mk` and `platform-core/packages/go-sdk/.golangci.yml` SHALL be the only copies of the shared make rules and lint configuration. A service SHALL reference them by relative path (`include ../platform-core/make/go-service.mk`; `golangci-lint run --config ../platform-core/packages/go-sdk/.golangci.yml`) and SHALL NOT hold copies. The shared `.golangci.yml` SHALL NOT contain a `service-local` block; a service-specific exclusion SHALL be a rule scoped by `path: team-<service>/...`. The goimports `local-prefixes` SHALL be `github.com/buidangphuc`.

#### Scenario: Shared lint file has no service-local block and one prefix
- **WHEN** `.golangci.yml` is parsed
- **THEN** it has no `BEGIN service-local` marker, and `local-prefixes` equals `github.com/buidangphuc`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestSharedLintConfigIsServiceNeutral`

#### Scenario: Service exclusions are path-scoped
- **WHEN** every exclusion rule of `.golangci.yml` that names a service is read
- **THEN** each carries a `path` that starts with `team-`, and a rule without a `path` does not name a service
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestServiceExclusionsArePathScoped`

#### Scenario: The template references the shared files
- **WHEN** the template `Makefile` is read
- **THEN** it includes `../platform-core/make/go-service.mk`, passes the shared lint config path, and the template directory has no `.golangci.yml` and no copy of `go-service.mk`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestTemplateReferencesSharedMakeAndLint`

### Requirement: The SDK has no dependency enable flags

The SDK SHALL NOT read an environment variable that switches a dependency on or off. Postgres, Kafka, the outbox, feature flags and consumers SHALL be always on when a service uses their package: `config.Postgres.Validate()` SHALL reject an empty `DATABASE_URL`, `config.Kafka` SHALL require `KAFKA_BROKERS` when `Runtime.IsStrict()`, and no `config` group SHALL declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED` or a consumer enable variable. `OTEL_ENABLED` SHALL remain, as the exporter switch.

#### Scenario: No config group declares a dependency enable variable
- **WHEN** every `env` tag of every group in `pkg/config` is listed
- **THEN** the only tag ending in `_ENABLED` is `OTEL_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestNoDependencyEnableFlags`

#### Scenario: Removed flags are ignored, not honoured
- **WHEN** settings embedding `Runtime` and `Postgres` load with `ENV=local`, `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false` and no `DATABASE_URL`
- **THEN** `Load` returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadIgnoresRemovedEnableFlags`

#### Scenario: No package source reads an enable flag
- **WHEN** the string literals of every non-test `.go` file under `pkg/` are scanned for names ending in `_ENABLED`
- **THEN** the only match is `OTEL_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestNoEnableFlagInSDKSource`

### Requirement: Mock data replaces only the outer adapter and never reaches a strict environment

The SDK SHALL provide the settings group `config.MockData` with variable `MOCK_DATA`, a comma-separated list of adapter names, and the package `pkg/mockdata`. `mockdata.New(cfg config.MockData, declared ...string) (*Set, error)` SHALL accept only names the service declared (compared case-insensitively after trimming, empty entries ignored) and SHALL return an error naming `MOCK_DATA` and the allowed names for an unknown name. `config.Load` SHALL refuse a non-empty `MOCK_DATA` when `Runtime.IsStrict()` with an error naming `MOCK_DATA`, from the same resolve step that runs before any group `Validate`, so no other validation error masks it. `Set.LogActive(logger)` SHALL log the active adapter names once per `Set` at boot and log nothing when none is active. `mockdata.Select(set, name, real, fixture)` SHALL return `fixture` when `name` is active and `real` otherwise. A mock adapter SHALL replace only the declared outermost adapter (payment provider, LLM backend, shipping or ETA provider); handlers, persistence, events and audit SHALL still run.

#### Scenario: Strict ENV refuses MOCK_DATA before other validation
- **WHEN** `ENV=prod`, `MOCK_DATA=payment-provider` and no `DATABASE_URL` is set for settings that embed `Runtime`, `Postgres` and `MockData`
- **THEN** `Load` returns an error naming `MOCK_DATA` and that error does not name `DATABASE_URL`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadRefusesMockDataInStrictEnvBeforeValidation`

#### Scenario: Every strict ENV spelling is refused
- **WHEN** `MOCK_DATA=payment-provider` is set with `ENV` equal to `staging`, `stage`, `prod`, `production` and `PROD`
- **THEN** each `Load` returns an error naming `MOCK_DATA`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestMockDataRefusedForEveryStrictSpelling`

#### Scenario: Local and test ENV accept MOCK_DATA, unset and empty are accepted everywhere
- **WHEN** `MOCK_DATA=payment-provider` is set with `ENV=local` and `ENV=test`, and `MOCK_DATA` is unset or empty with `ENV=prod`
- **THEN** every `Load` succeeds
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestMockDataAllowedWhenLocalOrEmpty`

#### Scenario: Unknown adapter name is refused
- **WHEN** `mockdata.New` receives `MOCK_DATA=payment-provider,telepathy` and the declared names `payment-provider` and `eta-provider`
- **THEN** it returns an error naming `MOCK_DATA`, `telepathy` and the declared names
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestNewRejectsUnknownAdapter`

#### Scenario: Names are trimmed, de-duplicated and case-insensitive
- **WHEN** `mockdata.New` receives ` Payment-Provider ,payment-provider,,` with `payment-provider` declared
- **THEN** `Set.Active("payment-provider")` is true and `Set.Adapters()` is `["payment-provider"]`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestNewNormalisesNames`

#### Scenario: Active adapters are logged once
- **WHEN** `LogActive` is called twice on a `Set` with `payment-provider` active, and once on a `Set` with none
- **THEN** exactly one record is logged for the first set, listing `payment-provider`, and none for the second
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestLogActiveOncePerSet`

#### Scenario: Select swaps only the named adapter
- **WHEN** `Select` runs for `payment-provider` active and for `eta-provider` inactive
- **THEN** the first returns the fixture and the second returns the real adapter
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestSelectSwapsOnlyActiveAdapter`

### Requirement: Privileged actions are audit-first and fail closed

`pkg/auditx` SHALL provide `config.Audit` with variable `UPSTREAM_AUDIT_ADDR`, which `Load` SHALL require (non-empty) when `Runtime.IsStrict()` and leave empty otherwise. `auditx.Dial(cfg config.Audit, serviceID string, scopes ...string) (*grpc.ClientConn, error)` SHALL return a lazy connection whose calls carry a service principal (`upstream.AsService`), and SHALL return a nil connection and no error when the address is empty. The SDK SHALL define `auditx.Event{ActorID, Action, TargetType, TargetID string; Metadata map[string]string}` and the interface `auditx.Writer` with `WriteAuditEvent(ctx, Event) error`, which a service implements over the generated `platform.audit.v1.AuditService.WriteAuditEvent` client (design.md D2). `auditx.NewGate(w Writer, logger *slog.Logger) *Gate` and `Gate.Do(ctx, ev, action func(context.Context) error) error` SHALL bound the write by the earlier of the caller deadline and `upstream.DefaultTimeout` (2s), write the event first and call `action` only when the write succeeded. When `w` is nil or the write returns an error, `Do` SHALL NOT call `action`, SHALL log only `action`, `target_type` and, for a write error, its gRPC status code (`status.Code(err)`), never `target_id` or the error text, and SHALL return a gRPC status with code `Unavailable` and message `audit trail unavailable`, which never contains the cause. An error returned by `action` SHALL be returned unchanged.

#### Scenario: A successful audit write lets the action run once
- **WHEN** `Gate.Do` is called with a writer that succeeds
- **THEN** the writer saw the event before `action` ran and `action` ran exactly once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoWritesBeforeAction`

#### Scenario: A failed audit write refuses the action
- **WHEN** `Gate.Do` is called with a writer that returns `errors.New("dial tcp 10.0.0.9: refused")`
- **THEN** `action` ran zero times, the code is `Unavailable`, the message is `audit trail unavailable` and does not contain `10.0.0.9`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedOnWriteError`

#### Scenario: A failed audit write logs no ids or error text
- **WHEN** `Gate.Do` is called with target id `listing-8f3a9c`, once with a writer that returns `status.Error(codes.Unavailable, "dial tcp 10.0.0.9: refused")` and once on a gate with a nil writer
- **THEN** both log records carry `action` and `target_type`, the first also carries code `Unavailable`, and neither contains `listing-8f3a9c`, `10.0.0.9` or the error text
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailureLogsNoTargetOrErrorText`

#### Scenario: A missing writer refuses the action
- **WHEN** `Gate.Do` is called on a gate built with a nil writer
- **THEN** `action` ran zero times and the status is `Unavailable` with message `audit trail unavailable`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedWithoutWriter`

#### Scenario: A timed-out audit write refuses the action
- **WHEN** the writer blocks until the context deadline of 50ms expires
- **THEN** `Do` returns `Unavailable` within 500ms and `action` ran zero times
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedOnTimeout`

#### Scenario: Without a caller deadline the write is bounded by 2s
- **WHEN** the context has no deadline and the writer blocks until its context ends
- **THEN** `Do` returns `Unavailable` within 2.5s and `action` ran zero times
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoBoundsWriteWithoutCallerDeadline`

#### Scenario: An action error is returned as is
- **WHEN** the audit write succeeds and `action` returns `status.Error(codes.PermissionDenied, "missing scope")`
- **THEN** `Do` returns that error with code `PermissionDenied`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoReturnsActionErrorUnchanged`

#### Scenario: Strict ENV requires the audit address
- **WHEN** settings embedding `Runtime` and `config.Audit` load with `ENV=prod` and `UPSTREAM_AUDIT_ADDR` unset or empty, and again with `ENV=local`
- **THEN** the first two `Load` calls return an error naming `UPSTREAM_AUDIT_ADDR` and the `local` one succeeds with an empty address
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestStrictEnvRequiresAuditAddr`

#### Scenario: Dial attaches a service principal
- **WHEN** a client made with `auditx.Dial(cfg, "team-order", "audit.write")` calls a test server
- **THEN** the server sees principal type `service`, id `team-order` and scope `audit.write`, and a dial with an empty address returns a nil connection and no error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDialSendsServicePrincipal`

### Requirement: Kafka consumers log a keyed key fingerprint, never the raw key

`config.LogKeyFP` SHALL read `LOG_KEY_FP_SECRET`; `Load` SHALL require it non-empty when `Runtime.IsStrict()` and otherwise fill a documented local default. `kafkax.NewFingerprinter(secret string) (*Fingerprinter, error)` SHALL reject an empty secret, and `Fingerprinter.Of(key []byte) string` SHALL return the first 12 lowercase hex characters of HMAC-SHA256 of `key` under the secret. `consume.Options` and `consume.BatchOptions` SHALL take an optional `KeyFP *kafkax.Fingerprinter`; when nil, a fingerprinter with a random per-process secret SHALL be used, never an unkeyed hash. Every log record that `consume` writes about a consumed record, a retry, a dead-lettering and a failed dead-letter write SHALL carry attribute `key_fp`, and SHALL NOT carry the raw key or the value in any attribute or message text. The record key bytes in the DLQ record and the offset and retry behaviour SHALL be unchanged. Wherever `consume` writes handler error text to a log record or to the `dlq-error` header, it SHALL replace every occurrence of the raw key text by `[key_fp:<fp>]`.

#### Scenario: Handler failure logs the fingerprint and not the key
- **WHEN** the handler fails on attempt 1 of 3 for key `seller-123` and the fingerprinter secret is `s3cret`
- **THEN** the log record carries `key_fp` equal to the first 12 hex of HMAC-SHA256(`s3cret`, `seller-123`) and the whole captured output does not contain `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestHandlerFailureLogsKeyFingerprint`

#### Scenario: Dead-letter logs carry the fingerprint and the DLQ key is unchanged
- **WHEN** the handler fails all attempts for key `seller-123` and the DLQ publish succeeds
- **THEN** the "record dead-lettered" log carries `key_fp` and no `key` attribute, and the DLQ record key equals the bytes `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestDeadLetterLogsFingerprintKeepsDLQKey`

#### Scenario: A failed dead-letter write still logs only the fingerprint
- **WHEN** the handler exhausts its attempts and the DLQ publish fails
- **THEN** the returned error and every log record do not contain `seller-123`, and the offset is not committed
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestDeadLetterFailureLogsFingerprintOffsetHeld`

#### Scenario: Error text embedding the key is scrubbed in logs and the DLQ header
- **WHEN** the handler returns `fmt.Errorf("user seller-123: %w", errBoom)` for key `seller-123` and the record is dead-lettered
- **THEN** the log `err` attribute and the `dlq-error` header equal `user [key_fp:<fp>]: boom` and contain no `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestErrorTextIsScrubbedOfKey`

#### Scenario: The batch consumer scrubs every key of the batch
- **WHEN** a batch handler fails with error text containing the keys `a-1` and `b-2` of two records
- **THEN** the logged text contains neither key and carries one `key_fp` per record
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestBatchLogsFingerprintsOnly`

#### Scenario: Fingerprints are 12 hex, stable per key and different across secrets
- **WHEN** `Of` runs twice for one key, once for another key (also a 200-byte key and a nil key), and once for the first key under a second secret
- **THEN** every result matches `^[0-9a-f]{12}$`, the first two are equal, the others differ, and the nil key does not panic
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestFingerprintFormatStableAndKeyed`

#### Scenario: An empty secret is refused
- **WHEN** `NewFingerprinter("")` runs
- **THEN** it returns an error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestNewFingerprinterRejectsEmptySecret`

#### Scenario: Strict ENV requires the secret and local has a default
- **WHEN** settings embedding `Runtime` and `config.LogKeyFP` load with `ENV=prod` and `LOG_KEY_FP_SECRET` unset or empty, and with `ENV=local`
- **THEN** the first two `Load` calls return an error naming `LOG_KEY_FP_SECRET` and `local` succeeds with a non-empty secret
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestStrictEnvRequiresLogKeyFPSecret`

#### Scenario: Without a configured fingerprinter the key is still not logged
- **WHEN** a consumer is built with `KeyFP` nil and the handler fails for key `seller-123`
- **THEN** the log carries a 12-hex `key_fp`, not an unkeyed SHA-256 of the key, and no `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestNilFingerprinterUsesRandomSecret`

### Requirement: Unique violations map to AlreadyExists

`grpcx.Status` SHALL map an error whose chain contains a Postgres error with SQLSTATE `23505` (unique violation) to `codes.AlreadyExists` with the fixed message `already exists`. A matching `ErrorTable` rule SHALL take precedence over this mapping, an error that already carries a gRPC status SHALL pass through first, and any other SQLSTATE SHALL keep becoming `Internal` with `internal error`. The constraint name, column and row detail SHALL be logged and SHALL NOT reach the caller.

#### Scenario: A unique violation becomes AlreadyExists
- **WHEN** `grpcx.Status` receives `fmt.Errorf("insert voucher: %w", &pgconn.PgError{Code: "23505", ConstraintName: "vouchers_code_key"})`
- **THEN** the code is `AlreadyExists` and the message is `already exists`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusMapsUniqueViolationToAlreadyExists`

#### Scenario: Constraint detail does not reach the caller
- **WHEN** the `PgError` carries `Detail: "Key (code)=(SAVE10) already exists."`
- **THEN** the status message does not contain `SAVE10` or `vouchers_code_key`, and the log record contains the constraint name
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusUniqueViolationHidesDetail`

#### Scenario: A service rule wins over the built-in mapping
- **WHEN** the error wraps both a table sentinel with `Rule{Code: FailedPrecondition}` and a 23505 `PgError`
- **THEN** the code is `FailedPrecondition`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusRuleBeatsUniqueViolation`

#### Scenario: Other database errors stay internal
- **WHEN** the error wraps a `PgError` with SQLSTATE `23503` (foreign key violation)
- **THEN** the code is `Internal` and the message is `internal error`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusOtherPgErrorsStayInternal`

#### Scenario: An existing gRPC status passes through before the mapping
- **WHEN** the error is `status.Error(codes.PermissionDenied, "missing scope")`
- **THEN** the code stays `PermissionDenied` and the message stays `missing scope`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusGRPCStatusBeatsUniqueViolation`
