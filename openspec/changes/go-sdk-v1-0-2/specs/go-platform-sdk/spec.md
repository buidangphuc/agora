## ADDED Requirements

### Requirement: The shared service chart can probe liveness on the liveness health service

The `platform-gitops/charts/service` value `probes.livenessService` SHALL default to the empty string. When `grpcPort` is greater than 0 and `probes.livenessService` is non-empty, the rendered gRPC `livenessProbe` SHALL carry `grpc.service` set to that value; the `readinessProbe` and `startupProbe` SHALL NOT carry `grpc.service` and SHALL keep probing the overall status. When the value is empty, the rendered container SHALL be identical to the one rendered before this change. An HTTP service (`healthPath`) SHALL be unaffected by the value. A service that adopts the SDK SHALL set `probes.livenessService: liveness` in its own values file; a service that has not adopted the SDK SHALL NOT set it.

#### Scenario: A gRPC service with the value set probes liveness by name
- **WHEN** `charts/service` is rendered for team-verification (`grpcPort` 50064) with `--set probes.livenessService=liveness`
- **THEN** the container `livenessProbe` is a `grpc:` probe with `service: liveness`, and the `readinessProbe` and `startupProbe` have no `service` key
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ops/gitops_render.feature` (helm template) scenario "A gRPC service with the value set probes liveness by name"

#### Scenario: The default render is unchanged
- **WHEN** `charts/service` is rendered for team-verification with its service values (it has not adopted the SDK), without `probes.livenessService`
- **THEN** the output is byte-identical to the golden render `platform-gitops/tests/golden/team-verification.pre-liveness.yaml` captured from the chart before this change, and no probe has a `service` key
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ops/gitops_render.feature` (helm template) scenario "The default render is unchanged"

#### Scenario: An HTTP service ignores the value
- **WHEN** `charts/service` is rendered for team-gateway (`healthPath=/healthz`, no `grpcPort`) with `--set probes.livenessService=liveness`
- **THEN** the container probes are `httpGet` probes and the rendered output contains no `service: liveness`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ops/gitops_render.feature` (helm template) scenario "An HTTP service ignores the value"

#### Scenario: The canary Rollout renders the same liveness probe
- **WHEN** `charts/service` is rendered for team-verification with `--set rollout.enabled=true --set probes.livenessService=liveness`
- **THEN** the Rollout container `livenessProbe` carries `service: liveness`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ops/gitops_render.feature` (helm template) scenario "The canary Rollout renders the same liveness probe"

### Requirement: gRPC servers share one interceptor chain, safe errors and opt-in reflection

`grpcx.NewServer(opts)` SHALL return a `*grpc.Server` with:
- the otelgrpc stats handler;
- the unary and stream chain request-id → recovery → logging → auth (request-id outermost so panic logs carry it);
- the health service from `opts.Health`;
- the reflection service only when `opts.Config.ReflectionEnabled` is true (`GRPC_REFLECTION_ENABLED`, default false; the strict-ENV refusal belongs to `config.Load`, see the scenario "Reflection is refused in a strict environment").

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

#### Scenario: Reflection is off by default
- **WHEN** a server is built with `Config.ReflectionEnabled` false and a client lists services over reflection
- **THEN** the call fails with `codes.Unimplemented`
- **VERIFIED BY**: `TestReflectionOffByDefault`

#### Scenario: Reflection is served locally when enabled
- **WHEN** a server is built with `Config.ReflectionEnabled` true and a client lists services over reflection
- **THEN** the response contains `grpc.health.v1.Health` and `grpc.reflection.v1.ServerReflection`
- **VERIFIED BY**: `TestReflectionServedWhenEnabled`

#### Scenario: Unknown errors do not leak
- **WHEN** a handler returns `grpcx.Status(ctx, logger, errors.New("pq: password=secret"), table)`
- **THEN** the caller sees `Internal` with message `internal error`
- **VERIFIED BY**: `TestStatusHidesUnknownErrors`

#### Scenario: Existing gRPC status errors pass through
- **WHEN** a handler returns `grpcx.Status(ctx, logger, status.Error(codes.PermissionDenied, "missing scope"), table)`
- **THEN** the caller sees `PermissionDenied` with message `missing scope`, not `Internal`
- **VERIFIED BY**: `TestStatusPassesThroughGRPCStatus`

## MODIFIED Requirements

### Requirement: Settings load from environment with the services' existing tags

`config.Load[T]()` SHALL populate a settings struct from environment variables. It reads `env:"NAME"` and `default:"value"` tags, the tag names the services use today, and recurses into nested structs without prefixing. It SHALL return an error naming every variable that fails to parse or is marked `required` and unset. After parsing, `Load` SHALL call `Validate() error` on `T` when it has one, and on every nested or embedded group that has one, and SHALL return all their errors joined. `config.Postgres.Validate()` SHALL reject an empty `URL`: the database has no enable flag. `config.Runtime.Validate()` SHALL reject an empty `ENV`. The SDK SHALL provide the embeddable groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox` and `Flags`. Their variable names and defaults SHALL be `ENV` (required, no default), `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT` (required), `SHUTDOWN_GRACE` (`10s`), `SHUTDOWN_DRAIN_DELAY` (`0s`), `GRPC_REFLECTION_ENABLED` (`false`), `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `KAFKA_BROKERS` (no tag default), `OUTBOX_POLL_INTERVAL` (`1s`), `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK` (`60s`), `FEATURE_FLAGS_ENABLED`, `FLIPT_ADDR` (no tag default) and `FEATURE_FLAGS_EVAL_TIMEOUT` (`500ms`). Every duration SHALL be a `time.Duration` field read as a Go duration, and no key carries a unit suffix. `KAFKA_BROKERS`, and `FLIPT_ADDR` while `FEATURE_FLAGS_ENABLED` is true, SHALL be required when `Runtime.IsStrict()` and default to `localhost:9092` and `localhost:9000` otherwise. `Load` SHALL enforce this by calling `ResolveFor(Runtime)` on `config.Kafka` and `config.Flags` with the `Runtime` it finds in the settings struct (embedded or a named field; the zero `Runtime`, so the local behaviour, when there is none); a `Settings` type needs nothing beyond holding `Runtime` and the group. `GRPC_REFLECTION_ENABLED` is a debug switch, not a dependency switch: `Load` SHALL call `ResolveFor(Runtime)` on `config.GRPC` and, when the value is true and `Runtime.IsStrict()`, return an error naming `GRPC_REFLECTION_ENABLED` before any `Validate` runs, and SHALL also return that error when the value is true and the settings struct holds no `Runtime`. There is no `DATABASE_ENABLED`. `Runtime.IsLocal()` SHALL be true only for an explicit `ENV` of `local` or `test`.

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

#### Scenario: Reflection is refused in a strict environment
- **WHEN** settings embedding `Runtime` and `GRPC` load with `ENV=prod`, `GRPC_PORT=50065` and `GRPC_REFLECTION_ENABLED=true`, and again with `ENV=staging`
- **THEN** `Load` returns an error naming `GRPC_REFLECTION_ENABLED` for each
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestStrictEnvRefusesGRPCReflection`

#### Scenario: Reflection without a Runtime is refused
- **WHEN** settings embedding `GRPC` but no `Runtime` load with `GRPC_PORT=50065` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** `Load` returns an error naming `GRPC_REFLECTION_ENABLED`, because the ENV cannot be proven non-strict
- **VERIFIED BY**: `TestReflectionWithoutRuntimeIsRefused`

#### Scenario: Reflection is accepted locally and defaults to off
- **WHEN** settings embedding `Runtime` and `GRPC` load with `ENV=local` and `GRPC_REFLECTION_ENABLED=true`, with `ENV=test` and the same value, and with `ENV=prod` and the variable unset
- **THEN** `GRPC.ReflectionEnabled` is true, true and false respectively and `Load` returns no error
- **VERIFIED BY**: `TestLocalEnvAcceptsGRPCReflection` and `TestGRPCReflectionDefaultsOff`

#### Scenario: A strict-env reflection refusal comes before group validation
- **WHEN** settings embedding `Runtime` and `GRPC` load with `ENV=prod`, `GRPC_REFLECTION_ENABLED=true` and `GRPC_PORT` unset
- **THEN** `Load` returns an error naming `GRPC_REFLECTION_ENABLED` and not `GRPC_PORT`
- **VERIFIED BY**: `TestStrictReflectionRefusalPrecedesValidation`

### Requirement: Health reflects dependency checks and drains on shutdown

`healthx.New(interval, logger) (*Health, error)` SHALL wrap the gRPC health server and SHALL return an error when interval is not positive. `Register(name, check func(ctx) error)` SHALL add checkers. `Run(ctx)` SHALL poll all checkers every interval, each bounded by the interval, and set the overall status to SERVING when all pass and NOT_SERVING when any fails. It SHALL log state transitions only. `Drain()` SHALL set NOT_SERVING permanently. `Health` SHALL also serve the health service name `liveness`, registered at `New` with status SERVING, which SHALL stay SERVING while the process runs regardless of any checker result and SHALL become NOT_SERVING only on `Drain()`. A service whose pod liveness probe names `liveness` is therefore never restarted because a dependency is down, while readiness (the overall `""` status) still follows the checks.

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

#### Scenario: Liveness stays serving while a dependency is down
- **WHEN** a registered checker fails and one interval elapses, and a client calls `Check` for service `liveness` and for service `""`
- **THEN** `liveness` is SERVING and `""` is NOT_SERVING
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/healthx` `TestLivenessIgnoresCheckerFailure`

#### Scenario: Liveness is serving from construction
- **WHEN** `New` returns and no `Run` has started
- **THEN** a `Check` for service `liveness` returns SERVING
- **VERIFIED BY**: `TestLivenessServingBeforeRun`

#### Scenario: A hanging checker does not delay liveness
- **WHEN** a checker blocks longer than the interval and a client calls `Check` for service `liveness`
- **THEN** the call returns SERVING within 5 ms
- **VERIFIED BY**: `TestLivenessAnswersFastDuringHangingChecker`

#### Scenario: Drain turns liveness off
- **WHEN** `Drain()` is called while all checkers pass
- **THEN** a `Check` for service `liveness` returns NOT_SERVING
- **VERIFIED BY**: `TestDrainSetsLivenessNotServing`

#### Scenario: An unknown health service name is still rejected
- **WHEN** a client calls `Check` for service `no-such-service`
- **THEN** the call fails with `codes.NotFound`
- **VERIFIED BY**: `TestUnknownHealthServiceNotFound`

### Requirement: The SDK has no dependency enable flags

The SDK SHALL NOT read an environment variable that switches a dependency on or off. Postgres, Kafka, the outbox, feature flags and consumers SHALL be always on when a service uses their package: `config.Postgres.Validate()` SHALL reject an empty `DATABASE_URL`, `config.Kafka` SHALL require `KAFKA_BROKERS` when `Runtime.IsStrict()`, and no `config` group SHALL declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED` or a consumer enable variable. `OTEL_ENABLED` SHALL remain, as the exporter switch, and `GRPC_REFLECTION_ENABLED` SHALL remain, as the debug switch of the gRPC reflection surface (off by default, refused when true while `Runtime.IsStrict()`); neither switches a dependency.

#### Scenario: No config group declares a dependency enable variable
- **WHEN** every `env` tag of every group in `pkg/config` is listed
- **THEN** the only tags ending in `_ENABLED` are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestNoDependencyEnableFlags`

#### Scenario: Removed flags are ignored, not honoured
- **WHEN** settings embedding `Runtime` and `Postgres` load with `ENV=local`, `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false` and no `DATABASE_URL`
- **THEN** `Load` returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadIgnoresRemovedEnableFlags`

#### Scenario: No package source reads an enable flag
- **WHEN** the string literals of every non-test `.go` file under `pkg/` are scanned for names ending in `_ENABLED`
- **THEN** the only matches are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestNoEnableFlagInSDKSource`

## REMOVED Requirements

### Requirement: Local-only database fallback

`postgres.OpenOrLocal(ctx, c config.Postgres, rt config.Runtime, logger) (*pgxpool.Pool, error)` SHALL return the pool when the database is reachable; when it is not, SHALL return `(nil, nil)` and log a warning only if `rt.IsLocal()` (`ENV` is `local` or `test`), and return the error otherwise. A nil logger SHALL default to `slog.Default()`.

**Reason**: A fallback to a nil pool lets a service boot without its database. Every service uses `postgres.Open`, and an unreachable database fails boot in every `ENV`.

**Migration**: Replace `postgres.OpenOrLocal(ctx, cfg, rt, logger)` with `postgres.Open(ctx, cfg)` and drop the nil-pool branches.

### Requirement: gRPC servers share one interceptor chain and safe errors

`grpcx.NewServer` registered reflection always and the scenario "Reflection is always on" required it.

**Reason**: Reflection is a debug surface that must be off unless asked for; the replacement requirement above has honest scenario titles.

**Migration**: Set `GRPC_REFLECTION_ENABLED=true` locally where reflection is wanted; never in staging or prod.
