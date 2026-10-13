# go-sdk

Shared platform layer for Go services: config, logging, tracing, auth, health,
lifecycle, Postgres, migrations, retries and the gRPC server chain.

Design and rules: [ADR-0016](../../docs/ADR/0016-go-service-convention-and-sdk.md).
Module: `github.com/buidangphuc/platform-core/packages/go-sdk`.

## Packages

All under `pkg/`.

| Package | What it does |
|---|---|
| `reqctx` | Request-id and principal-id accessors on `context.Context`. |
| `config` | `Load[T]()` from env tags: calls `Validate()` on `T` and on every nested or embedded group and joins the errors; `keepempty:"true"` keeps an explicitly empty value. Groups `Runtime` (`ENV` required; `IsLocal` only for `local`/`test`, `IsStrict`), `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Flags`. Durations are `time.Duration` fields read as Go durations (`10s`). Also `PositiveDuration` and the `CheckEnvExample` drift check. |
| `logx` | slog setup that adds request id, principal id and trace id from the context. |
| `otelx` | OTLP/gRPC tracer and meter providers; a no-op when disabled; always sets the W3C propagator. |
| `auth` | Principal from `x-principal-*` metadata, interceptors, `PrincipalType` constants (`TypeUser`, `TypeService`, `TypeAnonymous`), `RequirePrincipal`, `RequireScopes` (exact match), `RequireScopesAllowAnonymous`, `RequireService`, `RequireUser`. |
| `jwtx` | JWT claims and JWKS wire types shared by issuer and verifier. |
| `idempotency` | The `idempotency-key` metadata key, `Validate` and `ErrInvalidKey`. |
| `healthx` | `New(interval, logger) (*Health, error)` (interval must be positive); checker registry polled in the background; flips the gRPC health status; `Drain()` on shutdown. |
| `lifecycle` | Runs `Run(ctx) error` components with an ordered, deadline-bound shutdown, `Stack` for closers, `StopAfter`, `Every(interval, fn, opts...) (Component, error)` with the `RunAtStart` and `WithLogger` options, and `HTTP` (graceful HTTP server). Shutdown stays within one `ShutdownGrace` after the drain delay; `CloseGrace` (default a fifth of it, at most 2s) is reserved for the closers. |
| `postgres` | Pool open with ping, `OpenOrLocal` (nil pool only when `ENV` is local or test), health checker, `WithTx` (deferred, time-bounded rollback on error, panic and `runtime.Goexit`). |
| `migrate` | golang-migrate runner over a service's `migrations/` directory. |
| `pagination` | `Clamp` and opaque offset page tokens. |
| `grpcx` | Server builder (request-id, recovery, logging, auth, otelgrpc, health, reflection always on), serving that drains, and `Status` with an ordered `ErrorTable` (`NewErrorTable(Rule{Target, Code, Message}...) (ErrorTable, error)`, which rejects nil targets and `codes.OK`; first match wins; an empty `Message` sends the sentinel's text). Incoming `x-request-id` is kept only if it is at most 128 characters of `[A-Za-z0-9._-]`. |
| `kafkax` | franz-go producer (30s default delivery timeout) and consumer clients with trace and request-id headers; a group client uses `BlockRebalanceOnPoll`; an empty group gives a group-less reader; `Checker` for health. |
| `outbox` | Transactional outbox table (`SchemaSQL`), idempotent `EnqueueTx`, `Purge`, and a relayer that publishes through `Publisher.PublishRecord`, in order per aggregate, with retry and park. Rows carry `event_id`, `event_type`, `x-request-id` and `traceparent` headers and an optional per-row topic. |
| `consume` | `NewConsumer` and `NewBatchConsumer` (both return an error) with DLQ and `Permanent`; the handler ctx carries the trace and request id; one commit per fetch; `Record{Topic, Partition, Offset, Timestamp, Key, Value, Headers []Header}`; `NewDedupe` and `Dedupe` (`Seen`, `Processed`). |
| `upstream` | `NewClient`: lazy gRPC client with principal and request-id propagation, round_robin balancing, `AsService`, per-call service principal, `WithDialOptions`. |
| `resilience` | `Retry` and `Breaker` return `(Policy, error)` with safe defaults for zero options; `Do`; `Retryable` is an allow-list. `Policy` is opaque. |
| `flags` | OpenFeature client over Flipt that fails open; `BooleanEnabled(ctx, flag, def, opts...)` with `WithTargetingKey` and `WithAttributes`; `NewOrDisabled`. Built on `openfeature/isolated`, which upstream marks experimental. |
| `testkit` | Postgres and Redpanda containers, `SharedPostgres` + `Schema`, `FreePort`, `Eventually`, `Recorder`, `Produce`/`ReadAll`. |

`examples/hello` wires all of the above and is the reference service.

## How a service adopts it

1. Pick a release tag, for example `go-sdk/v1.0.0`.
2. From the repo root, sync the SDK into the service:

   ```sh
   platform-core/tools/sync-go-sdk.sh go-sdk/v1.0.0 team-<svc>
   ```

   This writes `third_party/go-sdk` (with `VERSION`), adds the `replace` to `go.mod`, copies `make/go-service.mk` and `.golangci.yml` (lines between `# BEGIN service-local` and `# END service-local` in the service's file are kept), pins the golangci-lint `tool` directive to the SDK's version, and runs `go mod tidy`. It fails without changes if the tag does not exist. If the tag has no `.golangci.yml`, it prints a warning and skips that file.
3. Copy `examples/hello` as the starting layout: `cmd/<binary>/main.go`, `internal/app`, `internal/config`, `internal/rpc`.
4. In the service `Makefile` set `SERVICE`, `GRPC_PORT` (and `BINARIES` if not `server`), then `include make/go-service.mk`. Run `make help` for the targets.
5. Copy `platform-core/templates/go-service/Dockerfile` to the service root. Build with `--build-arg BIN=<binary>`.

To upgrade, run the script again with the new tag and review the diff in a PR. Do not edit `third_party/go-sdk` by hand.

## Copying from examples/hello

The layout, wiring order and test conventions are in ADR-0016; `examples/hello` is the working version of them.

1. Copy `examples/hello` into the service and rename the module path in the imports, the binary under `cmd/`, and `const service` in `internal/app/app.go`.
2. Keep `cmd/<binary>/main.go` as is; it owns signals, settings and the exit code:

   ```go
   func main() {
   	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
   	defer stop()

   	s, err := config.Load()
   	if err != nil {
   		log.Fatal(err)
   	}
   	if err := app.Run(ctx, s); err != nil {
   		log.Fatal(err)
   	}
   }
   ```
3. In `internal/config`, keep `Settings`, `Validate`, `Load` and `TestEnvExampleInSync`; add service fields and their keys to `.env.example` at the service root.
4. Replace `internal/rpc/hello.go` (a hand-built `grpc.ServiceDesc`, used only to avoid codegen) with your generated registration, and `internal/greeter` with your domain packages. Handlers stay thin: `auth.Require*`, domain call, `grpcx.Status` with your `ErrorTable`. Reflection is always on; it only describes methods once the service has generated descriptors.
5. Drop `internal/store` and `migrations/` if the service has no database. Otherwise keep migrations in `<repo>/migrations/` (`NNNN_name.up.sql`/`.down.sql` plus `migrations/embed.go` with `//go:embed *.sql`), applied by the deploy migrate job, not at boot; tests and an optional `migrate up` subcommand pass `migrations.Files` to `migrate.Up`. Add `internal/events` only when needed.
6. Set goimports `local-prefixes` to your module path (the synced `.golangci.yml` has the SDK's value).
7. Tests: see ADR-0016 ("Tests") for ports, `grpcx.ServeListener`, `logx.NewTo` and `GO_SDK_REQUIRE_DOCKER`.

## Upgrading to v1

v1 changes API and configuration on purpose; there are no deprecated aliases. The sections below list every breaking change since `go-sdk/v0.2.1` (what most services vendor). A service on `v0.1.1` also takes the "from v0.1.1" list. Re-run `platform-core/tools/sync-go-sdk.sh go-sdk/<tag> <service>` and fix the compile errors with this list.

### Renames

| Before | After |
|---|---|
| import `pkg/pgx`, package `pgx` | `pkg/postgres`, package `postgres` (no more `jpgx` alias next to jackc/pgx) |
| `upstream.Dial(addr, opts...)` | `upstream.NewClient(addr, opts...)` |
| `Principal.Type string` | `auth.PrincipalType`; compare with `auth.TypeUser`, `TypeService`, `TypeAnonymous` (convert with `string(...)` where a plain string is needed) |

### Configuration

- `ENV` is required and has no default; it used to default to `local`. An unset `ENV` now fails `Load`, and `IsLocal()` is true only for `local` or `test`. Add `ENV=` to every deployment and to `.env.example`. `OpenOrLocal` falls back to a nil pool only when `IsLocal()`.
- `Load` calls `Validate()` on every nested or embedded group and joins the errors. A `Settings` that wrapped `Postgres.Validate()` by hand can drop it; one that embeds several groups no longer needs to pick which `Validate` is promoted.
- `KAFKA_BROKERS` and, with flags enabled, `FLIPT_ADDR` lose their tag defaults and are required when `ENV` is staging, stage, prod or production (`Runtime.IsStrict`); a local or test `ENV` still gets `localhost:9092` and `localhost:9000`. `Load` applies this through `ResolveFor(Runtime)` on `Kafka` and `Flags`, using the `Runtime` embedded in (or held by) your `Settings`; add `KAFKA_BROKERS` and `FLIPT_ADDR` to every deployment that uses them.
- Every duration is a Go duration (`time.Duration` fields, values like `30s` or `500ms`) and the keys lose their unit suffix. Defaults keep their value.

| Old key (field) | New key (field) | Default |
|---|---|---|
| `SHUTDOWN_GRACE_SECONDS` (`GRPC.ShutdownGraceSeconds float64`, method `ShutdownGrace()`) | `SHUTDOWN_GRACE` (`GRPC.ShutdownGrace`) | `10s` |
| `SHUTDOWN_DRAIN_DELAY_SECONDS` (`GRPC.DrainDelaySeconds float64`, method `DrainDelay()`) | `SHUTDOWN_DRAIN_DELAY` (`GRPC.DrainDelay`) | `0s` |
| `OUTBOX_CLAIM_LOCK_SECONDS` (`Outbox.ClaimLockSeconds int`) | `OUTBOX_CLAIM_LOCK` (`Outbox.ClaimLock`) | `60s` |
| `FEATURE_FLAGS_EVAL_TIMEOUT_MS` (`Flags.EvalTimeoutMS int`) | `FEATURE_FLAGS_EVAL_TIMEOUT` (`Flags.EvalTimeout`) | `500ms` |

  `OUTBOX_POLL_INTERVAL` was already a Go duration. The `ShutdownGrace()` and `DrainDelay()` methods are gone: read the fields. Reflection is always on and the database has no enable flag; neither `GRPC_REFLECTION_ENABLED` nor `DATABASE_ENABLED` exists.

### API signatures

| Package | Before | After |
|---|---|---|
| `grpcx` | `ErrorTable map[error]codes.Code` | `NewErrorTable(Rule{Target, Code, Message}...) (ErrorTable, error)`, ordered, first `errors.Is` match wins. An empty `Message` sends `Target.Error()`, not the wrapped chain. `context.Canceled` and `DeadlineExceeded` map to their codes and log at Info. |
| `grpcx` | any `x-request-id` accepted | kept only if at most 128 characters of `[A-Za-z0-9._-]`, else a new id is generated |
| `healthx` | `New(interval, logger) *Health` | `(*Health, error)`; the interval must be positive |
| `lifecycle` | (new in v1) | `Every(interval, fn, opts...) (Component, error)`; shutdown of components and closers shares one `ShutdownGrace`, with `Options.CloseGrace` reserved for the closers |
| `consume` | `NewConsumer(...) *Consumer` | `NewConsumer(...) (*Consumer, error)`; `Options.DLQTopic` is required |
| `consume` | `NewDedupe(table) Dedupe`, `DedupeSchemaSQL(table) string`, `Seen(ctx, pgx.Tx, ...)` | `NewDedupe(table) (Dedupe, error)`, `DedupeSchemaSQL(table) (string, error)`, `Seen(ctx, Executor, ...)`, new `Processed(ctx, Querier, ...)` |
| `consume` | `Record{Topic, Key, Value, Headers map[string]string, Offset}` | `Record{Topic, Partition, Offset, Timestamp, Key, Value, Headers []Header}`; use `Record.Header(key)`. Duplicate headers are kept. DLQ records carry source partition, offset and group headers. |
| `consume` | handler ctx is the poll loop's; commit after every record | handler ctx carries the W3C trace context and request id of the record (batch handlers get a span linked to each record); offsets are committed once per fetch |
| `consume` | (batch consumer new) | `NewBatchConsumer(...) (*BatchConsumer, error)`; a failing batch retries `BatchOptions.MaxAttempts` times (default 5), then goes to the DLQ |
| `outbox` | `Publisher.Publish(ctx, topic, key, value)` | `Publisher.PublishRecord(ctx, *kgo.Record)`; `kafkax.Producer` satisfies both. Records carry headers `event_id`, `event_type`, `x-request-id` and `traceparent` (stored at enqueue). |
| `outbox` | any order; a failed row did not hold back later rows | per aggregate in order: a row is not claimed while an older unpublished row of the same aggregate exists (a parked row blocks its aggregate until handled). The relay batch is bounded by half the claim lock. |
| `outbox` | `Row` had no topic | optional `Row.Topic` overrides `RelayerOptions.Topic`; an empty `EventID` returns `ErrEmptyEventID`; new `Store.Purge(ctx, olderThan)`. The `principal` column is no longer written. |
| `resilience` | `Retry(o) failsafe.Policy[any]`, `Breaker(o) failsafe.Policy[any]`, `Do(ctx, fn, ...failsafe.Policy[any])` | `Retry(o) (Policy, error)`, `Breaker(o) (Policy, error)`, `Do(ctx, fn, ...Policy)` with an SDK `Policy`. Zero options now take defaults (3 attempts, 100ms to 5s backoff, breaker threshold 5, delay 30s); negative values are rejected; `Do` returns an error for a zero `Policy`. |
| `resilience` | `Retryable` was a deny-list: plain errors and Internal were retried | allow-list: only Unavailable, ResourceExhausted, Aborted and DeadlineExceeded are retried; everything else is not. Pass `RetryOptions.RetryIf` to change that. |
| `flags` | `BooleanEnabled(ctx, flag, def)`; targeting key fixed to `"system"` | `BooleanEnabled(ctx, flag, def, opts ...EvalOption)` on `Evaluator`; the key defaults to the principal id in ctx (`anonymous` without one); `WithTargetingKey`, `WithAttributes`. Fakes that implement `Evaluator` need the new signature. `Close` shuts down only its own provider. `NewWithProvider(ctx, p, timeout, logger)` takes the same leading arguments as `New`. |
| `kafkax` | `NewConsumerClient` | group clients are built with `kgo.BlockRebalanceOnPoll`; `NewProducer` sets a 30s `RecordDeliveryTimeout` by default |

`BlockRebalanceOnPoll` needs a call to `Client.AllowRebalance()` after each fetch is committed, and before `Close`. `consume.Consumer` and `consume.BatchConsumer` do it. A service that polls a group client from `kafkax.NewConsumerClient` directly must call `AllowRebalance()` itself, or the group stops rebalancing and `Close` blocks.

### Outbox table migration

The relayer needs three new columns and two indexes. `outbox.SchemaSQL(table)` adds them on a table created by an older SDK (it is idempotent). A service that applies its own migrations must run this before deploying v1:

```sql
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS seq         BIGSERIAL;
CREATE INDEX IF NOT EXISTS <table>_agg_idx ON <table> (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX IF NOT EXISTS <table>_pending_idx ON <table> (seq) WHERE status = 'pending';
```

`seq` orders events: `created_at` is the transaction start, so events enqueued in one transaction tie on it. The old `principal` column may stay or be dropped.

### From v0.1.1

Everything above applies, plus these packages first appeared after v0.1.1 and need no migration, only adoption: `consume`, `kafkax`, `outbox`, `upstream`, `flags`. The v0.1.1 `GRPC`, `Postgres` and `OTel` groups kept their keys apart from the renames above, and `GRPC.Validate()` now rejects a zero `GRPC_PORT`.

### Notes

- `flags` uses `github.com/open-feature/go-sdk/openfeature/isolated`, which upstream marks experimental. It gives each client its own provider and lets `Close` stop only that one; a future upstream change may need a small adaptation in `flags`.

## Module layout

`testkit` and the golangci-lint `tool` directive stay in the SDK module. Moving them to separate modules was evaluated (review finding m16) and rejected for v1:

- `sync-go-sdk.sh` vendors the SDK directory at a tag into `third_party/go-sdk` with one `replace`, and reads the golangci-lint version from the `tool` directive in the SDK's `go.mod`. A second module for `testkit` would need its own `replace` and placeholder require in every service, and a second module for the tooling would need a different source for the lint version; both change `sync-go-sdk.sh`, `check-go-sdk-vendor.sh` and their tests beyond trivial edits.
- The cost they would save is only module-graph size: `go mod tidy` in a service prunes what its packages do not import, so a service that does not import `testkit` does not build testcontainers.

Revisit if the vendored graph causes real trouble.
