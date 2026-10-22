# go-sdk

Version: v1.0.1

Shared platform layer for Go services: config, logging, tracing, auth, health,
lifecycle, Postgres, migrations, retries and the gRPC server chain.

Design and rules: [ADR-0016](../../docs/ADR/0016-go-service-convention-and-sdk.md).
Module: `github.com/buidangphuc/agora/platform-core/packages/go-sdk`.

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
| `outbox` | Transactional outbox table (`SchemaSQL`), idempotent `EnqueueTx`, `Purge`, and a relayer that publishes through `Publisher.PublishRecord`, in order per aggregate, with capped-backoff retry (transient errors never park) and parking of record-level errors only (`Permanent`, `Store.Requeue`, `Store.RequeueParked`, `Store.Stats`). Rows carry the `kafkax` headers `event-id`, `event-type`, `x-request-id` and `traceparent` and an optional per-row topic. |
| `consume` | `NewConsumer` and `NewBatchConsumer` (both return an error) with DLQ and `Permanent`; the handler ctx carries the trace and request id; one commit per fetch; `Record{Topic, Partition, Offset, Timestamp, Key, Value, Headers []Header}`; `NewDedupe` and `Dedupe` (`Seen`, `Processed`). |
| `upstream` | `NewClient`: lazy gRPC client with principal and request-id propagation, round_robin balancing, `AsService`, per-call service principal, `WithDialOptions`. |
| `resilience` | `Retry` and `Breaker` return `(Policy, error)` with safe defaults for zero options; `Do`; `Retryable` is an allow-list. `Policy` is opaque. |
| `flags` | OpenFeature client over Flipt that fails open; `BooleanEnabled(ctx, flag, def, opts...)` with `WithTargetingKey` and `WithAttributes`; `NewOrDisabled`. Built on `openfeature/isolated`, which upstream marks experimental. |
| `testkit` | Postgres and Redpanda containers, `SharedPostgres` + `Schema`, `FreePort`, `Eventually`, `Recorder`, `Produce`/`ReadAll`. |

`examples/hello` wires all of the above and is the reference service.

## How a service adopts it

1. Pick a release tag, for example `go-sdk/v1.0.1`.
2. Require it in the service `go.mod`:

   ```sh
   go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1
   ```

   No `replace` and no vendored copy. The service Makefile includes `../platform-core/make/go-service.mk` and passes `../platform-core/packages/go-sdk/.golangci.yml` as the lint config.
3. Copy `examples/hello` as the starting layout: `cmd/<binary>/main.go`, `internal/app`, `internal/config`, `internal/rpc`.
4. In the service `Makefile` set `SERVICE`, `GRPC_PORT` (and `BINARIES` if not `server`), then `include ../platform-core/make/go-service.mk`. Run `make help` for the targets.
5. Copy `platform-core/templates/go-service/Dockerfile` to the service root. Build with `--build-arg BIN=<binary>`.

To upgrade, `go get` the new tag and review the diff in a PR.

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

v1 changes API and configuration on purpose; there are no deprecated aliases. The sections below list every breaking change since `go-sdk/v0.2.1` (what most services vendor). A service on `v0.1.1` also takes the "from v0.1.1" list. Run `go get` for the new tag and fix the compile errors with this list.

### Renames

| Before | After |
|---|---|
| import `pkg/pgx`, package `pgx` | `pkg/postgres`, package `postgres` (no more `jpgx` alias next to jackc/pgx) |
| `upstream.Dial(addr, opts...)` | `upstream.NewClient(addr, opts...)` |
| `Principal.Type string` | `auth.PrincipalType`; compare with `auth.TypeUser`, `TypeService`, `TypeAnonymous` (convert with `string(...)` where a plain string is needed) |

### Configuration

- `ENV` is required and has no default; it used to default to `local`. An unset `ENV` now fails `Load`, and `IsLocal()` is true only for `local` or `test`. Add `ENV=` to every deployment and to `.env.example`. `OpenOrLocal` falls back to a nil pool only when `IsLocal()`.
- `Load` calls `Validate()` on every nested or embedded group and joins the errors. A `Settings` that wrapped `Postgres.Validate()` by hand can drop it; one that embeds several groups no longer needs to pick which `Validate` is promoted.
- `KAFKA_BROKERS` loses its tag default and is required when `ENV` is staging, stage, prod or production (`Runtime.IsStrict`); a local or test `ENV` still gets `localhost:9092`. `Load` applies this through `ResolveFor(Runtime)` on `Kafka`, using the `Runtime` embedded in (or held by) your `Settings`; add `KAFKA_BROKERS` to every deployment that uses it.
- `FEATURE_FLAGS_ENABLED` and `config.Flags.Enabled` are removed: flags have no on/off switch. A service always evaluates through Flipt at `FLIPT_ADDR` (default `localhost:9000`), and an unreachable Flipt makes every evaluation return the default written in code. Drop the variable from env files and manifests; set `FLIPT_ADDR` where Flipt runs.
- Every duration is a Go duration (`time.Duration` fields, values like `30s` or `500ms`) and the keys lose their unit suffix. Defaults keep their value.

| Old key (field) | New key (field) | Default |
|---|---|---|
| `SHUTDOWN_GRACE_SECONDS` (`GRPC.ShutdownGraceSeconds float64`, method `ShutdownGrace()`) | `SHUTDOWN_GRACE` (`GRPC.ShutdownGrace`) | `10s` |
| `SHUTDOWN_DRAIN_DELAY_SECONDS` (`GRPC.DrainDelaySeconds float64`, method `DrainDelay()`) | `SHUTDOWN_DRAIN_DELAY` (`GRPC.DrainDelay`) | `0s` |
| `OUTBOX_CLAIM_LOCK_SECONDS` (`Outbox.ClaimLockSeconds int`) | `OUTBOX_CLAIM_LOCK` (`Outbox.ClaimLock`) | `60s` |
| `FEATURE_FLAGS_EVAL_TIMEOUT_MS` (`Flags.EvalTimeoutMS int`) | `FEATURE_FLAGS_EVAL_TIMEOUT` (`Flags.EvalTimeout`) | `500ms` |
| `OUTBOX_MAX_ATTEMPTS` (`Outbox.MaxAttempts int`) | removed: the relayer has no attempt limit; use `RelayerOptions.AlertAfter` | |

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
| `outbox` | `Publisher.Publish(ctx, topic, key, value)` | `Publisher.PublishRecord(ctx, *kgo.Record)`; `kafkax.Producer` satisfies both. Records carry the `kafkax` header constants `event-id`, `event-type`, `x-request-id` and `traceparent` (stored at enqueue); the v0 names `event_id` and `event_type` are gone, and the outbox has no header constants of its own. |
| `outbox` | any order; a failed row did not hold back later rows | per aggregate in order: a row is not claimed while an older unpublished row of the same aggregate exists (a parked row blocks its aggregate until `Store.Requeue`). One pass publishes up to `BatchSize` consecutive rows of an aggregate and stops that aggregate at its first failure; an aggregate whose head is parked or backing off is never scanned. The relay batch is bounded by half the claim lock, and each mark has its own `RelayerOptions.MarkTimeout`. |
| `outbox` | any publish error counted toward `MaxAttempts`, then parked | every error retries forever with capped exponential backoff, logged at ERROR once `RelayerOptions.AlertAfter` (default 10) attempts have failed; only an error permanent for the record parks at once: `MessageTooLarge`, `RecordListTooLarge`, `CorruptMessage`, `InvalidRecord`, `InvalidTimestamp`, or any error a custom `Publisher` wraps in `outbox.Permanent`. Unknown topic, authorization and other topic or cluster errors retry, so one missing topic cannot park every aggregate. `Store.RequeueParked(ctx, RequeueFilter{Topic, ErrorContains})` requeues parked rows in bulk. The head scan rotates through the aggregates (cursor with wrap-around), so a busy key cannot starve the others. A budget that expires mid-publish releases the row without counting an attempt. A row with no topic is logged at ERROR and pushed back by one `ClaimLock`, without an attempt. `NewRelayer` rejects `3*MarkTimeout >= ClaimLock/2`; the default `MarkTimeout` is the smaller of 10s and `ClaimLock/8`. |
| `outbox` | `NewStore(pool, table) *Store` and `NewRelayer(...) *Relayer` panic or do not validate; `SchemaSQL(table) string` | `NewStore(pool, table) (*Store, error)`, `NewRelayer(...) (*Relayer, error)` (nil store or publisher, negative or inconsistent options are errors), `SchemaSQL(table) (string, error)`; new `Store.Requeue(ctx, eventID)`, `Store.Stats(ctx)`, `Store.RequeueParked`. Table names are at most 41 characters. |
| `outbox` | `Row` had no topic | optional `Row.Topic` overrides `RelayerOptions.Topic`; an empty `EventID` returns `ErrEmptyEventID`; new `Store.Purge(ctx, olderThan)`. The `principal` column is no longer written. |
| `resilience` | `Retry(o) failsafe.Policy[any]`, `Breaker(o) failsafe.Policy[any]`, `Do(ctx, fn, ...failsafe.Policy[any])` | `Retry(o) (Policy, error)`, `Breaker(o) (Policy, error)`, `Do(ctx, fn, ...Policy)` with an SDK `Policy`. Zero options now take defaults (3 attempts, 100ms to 5s backoff, breaker threshold 5, delay 30s); negative values are rejected; `Do` returns an error for a zero `Policy`. |
| `resilience` | `Retryable` was a deny-list: plain errors and Internal were retried | allow-list: only Unavailable, ResourceExhausted, Aborted and DeadlineExceeded are retried; everything else is not. Pass `RetryOptions.RetryIf` to change that. |
| `flags` | `BooleanEnabled(ctx, flag, def)`; targeting key fixed to `"system"` | `BooleanEnabled(ctx, flag, def, opts ...EvalOption)` on `Evaluator`; the key defaults to the principal id in ctx (`anonymous` without one); `WithTargetingKey`, `WithAttributes`. Fakes that implement `Evaluator` need the new signature. `Close` shuts down only its own provider. `NewWithProvider(ctx, p, timeout, logger)` takes the same leading arguments as `New`. |
| `kafkax` | `NewConsumerClient` | group clients are built with `kgo.BlockRebalanceOnPoll`; `NewProducer` sets a 30s `RecordDeliveryTimeout` by default |

`BlockRebalanceOnPoll` needs a call to `Client.AllowRebalance()` after each fetch is committed, and before `Close`. `consume.Consumer` and `consume.BatchConsumer` do it. A service that polls a group client from `kafkax.NewConsumerClient` directly must call `AllowRebalance()` itself, or the group stops rebalancing and `Close` blocks.

### Outbox table migration

A v0 table needs the columns `topic`, `traceparent` and `seq`, two new indexes, and loses `claim_idx`. `outbox.SchemaSQL(table)` creates a table for a service that has none; applied to a v0 table it fails on the `seq` index and changes nothing that matters. Do not add `seq` as `BIGSERIAL`: that rewrites the table under ACCESS EXCLUSIVE and numbers the old rows in heap order, so a retried row (rewritten by an update) would move behind newer events of its aggregate.

Before you start, in this order:

1. Stop the v0 relayers (or let them drain the pending rows): a v0 relayer ignores `seq`.
2. Deal with rows a v0 relayer parked. v0 parked a row as `failed` after `OUTBOX_MAX_ATTEMPTS` attempts; v1 never sends a `failed` row but keeps it as the head of its aggregate, so it blocks every later event of that aggregate. List them with `SELECT event_id, aggregate_type, aggregate_id, attempts, error FROM <table> WHERE status = 'failed'`. Fix the cause, then either `DELETE` the rows you will not send or put the rest back, which works on the v0 table as it is: `UPDATE <table> SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL WHERE status = 'failed'`. After the migration, `Store.RequeueParked(ctx, outbox.RequeueFilter{})` does the same for the rows v1 parks later, and can be narrowed by topic or error text.
3. `Store.Purge` the published rows you do not need, or `DELETE FROM <table> WHERE status = 'published'`. Published rows need no ordered numbers, and every Backfill batch sorts all rows that are still unnumbered, so fewer rows means faster batches.

Then run these steps; `outbox.UpgradeSQL(table)` returns the same statements and the order (`Prepare` and `Finish` statement by statement, `Backfill` until `updated` is 0, `Switch` in one transaction):

```sql
-- 1. columns and sequence (instant)
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE <table> ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS <table>_seq_seq OWNED BY <table>.seq;

-- 2. number the old rows by (created_at, event_id), 10000 at a time; repeat until updated = 0.
--    Numbers come from the sequence in that order and each batch locks its rows,
--    so two runs at once cannot number a row twice or out of order.
WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
       (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
     u AS (UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
SELECT count(*) AS updated FROM u;

-- 3. switch, one transaction: block writers, number the rows a v0 writer inserted since
--    the last batch, give new rows a number. No row is left without a seq afterwards.
BEGIN;
LOCK TABLE <table> IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
       (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE <table> ALTER COLUMN seq SET DEFAULT nextval('<table>_seq_seq');
COMMIT;

-- 4. NOT NULL without a long scan under ACCESS EXCLUSIVE (the first line makes a rerun safe)
ALTER TABLE <table> DROP CONSTRAINT IF EXISTS <table>_seq_nn;
ALTER TABLE <table> ADD CONSTRAINT <table>_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
ALTER TABLE <table> VALIDATE CONSTRAINT <table>_seq_nn;
ALTER TABLE <table> ALTER COLUMN seq SET NOT NULL;
ALTER TABLE <table> DROP CONSTRAINT <table>_seq_nn;

-- 5. indexes, not in a transaction; if one fails, DROP the invalid index and rerun
CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_status_idx  ON <table> (status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_agg_idx     ON <table> (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_pending_idx ON <table> (seq) WHERE status = 'pending';
DROP INDEX CONCURRENTLY IF EXISTS <table>_claim_idx;
```

Do not run step 2 again after step 3. The switch blocks inserts into the table for as long as it takes to number the rows still unnumbered, normally a moment; v0 writers wait on that lock and then get a `seq` from the new default.

`seq` orders events: `created_at` is the transaction start, so events enqueued in one transaction tie on it. The old `principal` column may stay or be dropped. Deploy v1 once step 5 is done.

Parked rows: alert on `Store.Stats(ctx).Parked` (a parked row blocks its aggregate). After fixing the cause, `Store.Requeue(ctx, eventID)` resets one row; `Store.RequeueParked(ctx, outbox.RequeueFilter{Topic: ..., ErrorContains: ...})` resets every parked row that matches. Do not edit rows by hand. Only an error permanent for the record parks a row (`MessageTooLarge`, `RecordListTooLarge`, `CorruptMessage`, `InvalidRecord`, `InvalidTimestamp`, or one a custom `Publisher` wraps in `outbox.Permanent`); everything else retries and is logged at ERROR after `RelayerOptions.AlertAfter` attempts.

### From v0.1.1

Everything above applies, plus these packages first appeared after v0.1.1 and need no migration, only adoption: `consume`, `kafkax`, `outbox`, `upstream`, `flags`. The v0.1.1 `GRPC`, `Postgres` and `OTel` groups kept their keys apart from the renames above, and `GRPC.Validate()` now rejects a zero `GRPC_PORT`.

### Notes

- `flags` uses `github.com/open-feature/go-sdk/openfeature/isolated`, which upstream marks experimental. It gives each client its own provider and lets `Close` stop only that one; a future upstream change may need a small adaptation in `flags`.

## Module layout

`testkit` and the golangci-lint `tool` directive stay in the SDK module. Moving them to separate modules was evaluated (review finding m16) and rejected for v1:

- A second module for `testkit` or the tooling would need its own require in every service and its own release tag.
- The cost they would save is only module-graph size: `go mod tidy` in a service prunes what its packages do not import, so a service that does not import `testkit` does not build testcontainers.

Revisit if the vendored graph causes real trouble.
