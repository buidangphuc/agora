# Staff review of go-sdk before v1.0.0 (2026-10-13)

Verdict: ready after fixes. Owner decisions: fix every finding; one duration format (Go duration) for env vars, renaming the keys; rename package `pgx` to `postgres` and `upstream.Dial` to `upstream.NewClient`; the outbox guarantees per-aggregate order.

Paths are under `platform-core/packages/go-sdk/`.

| id | pkg | where | problem | fix |
|---|---|---|---|---|
| B1 | config, pgx | config/groups.go:14,20; pgx/pgx.go:52 | `ENV` defaults to `local`, so an unset ENV is local and `OpenOrLocal` falls back to in-memory | `ENV` required, no default; `IsLocal` only for explicit `local`/`test` |
| B2 | outbox | relayer.go:12-14,88; store.go:27-34,66,86 | `Publisher.Publish(ctx, topic, key, value)` carries no headers; `request_id` written but never read; `principal` column unused; `Row` has no topic so two relayers on one table cross-publish; relay ctx has no request id/trace | `Publisher` = `PublishRecord(ctx, *kgo.Record) error` (same shape as `consume.DLQProducer`); headers `event_id`, `event_type`, `x-request-id`, stored `traceparent`; nullable per-row `topic` column (relayer default topic otherwise); drop or use `principal` |
| M1 | consume | consume.go:125; batch.go:90 | handler gets the poll-loop ctx; `traceparent`/`x-request-id` never extracted | per-record ctx: propagator extract + `reqctx.WithRequestID`; span links for batches |
| M2 | consume, kafkax | consume.go:110; kafkax.go:87 | synchronous per-record commit; no rebalance handling | commit once per fetch; `BlockRebalanceOnPoll` + `AllowRebalance` (or commit in `OnPartitionsRevoked`) |
| M3 | consume | batch.go:89-102 | batch retries a non-Permanent error forever | `BatchOptions.MaxAttempts` (default 5), then DLQ |
| M4 | consume | consume.go:50,73; batch.go:21,38 | `DLQTopic` required but unvalidated; constructors cannot fail | `NewConsumer`/`NewBatchConsumer` return `(…, error)` and validate |
| M5 | outbox, kafkax | relayer.go:88; kafkax.go:32-37 | no delivery timeout: broker outage blocks Publish forever, lease expiry causes duplicate publish | default `RecordDeliveryTimeout` (30s) in `NewProducer`; bound each relay batch by a ctx shorter than the claim lock |
| M6 | outbox | store.go:96-98; relayer.go:114 | a failed row backs off while later rows of the same aggregate publish | head-of-line per aggregate: do not claim a row while an older unpublished row of the same aggregate exists |
| M7 | pgx | pgx.go:65-80 | `WithTx` has no deferred rollback; a panic leaks the tx and the connection | `defer tx.Rollback` (or `pgx.BeginFunc`) |
| M8 | config | config.go:32; groups.go:46,73; examples/hello/internal/config/config.go:19 | `Load` only calls `Validate` on T; embedded groups' `Validate` are ambiguous and skipped | `Load` walks nested groups and joins every `Validate()`; fix the ADR example |
| M9 | resilience | resilience.go:39-54 | `RetryOptions{}` retries unbounded with no delay; `BreakerOptions{}` panics (divide by zero) | defaults (3 attempts, 100ms/5s, threshold 5, delay 30s); reject invalid values |
| M10 | resilience | resilience.go:65-75 | `Retryable` is a deny-list: non-gRPC errors and Internal are retried | allow-list: Unavailable, ResourceExhausted, Aborted, DeadlineExceeded |
| M11 | resilience | resilience.go:34,50,59 | public API exposes `failsafe.Policy[any]` from failsafe-go v0 | opaque SDK `Policy` type |
| M12 | grpcx | status.go:37-53 | `context.Canceled`/`DeadlineExceeded` become Internal with an ERROR log | `status.FromContextError` first; log at Info/Warn |
| M13 | flags | flags.go:24-27,102 | targeting key hard-coded to "system"; no attributes | evaluation context argument; default targeting key from `auth.FromContext` |
| M14 | grpcx | status.go:25,40 | `ErrorTable map[error]Rule` matches in random order | ordered table (`[]Rule{Target, Code, Message}` or `NewErrorTable(...)`) |
| m1 | config | groups.go:37,40,106-117 | four duration encodings (float s, Go duration, int s, int ms) | Go duration everywhere; rename keys (`SHUTDOWN_GRACE`, `OUTBOX_CLAIM_LOCK`, `FEATURE_FLAGS_EVAL_TIMEOUT`, …) |
| m2 | spec | openspec/specs/go-platform-sdk/spec.md; grpcx/doc.go:2 | main spec still promises `GRPC_REFLECTION_ENABLED`, `DATABASE_ENABLED`, old signatures; doc says "optional reflection" | MODIFIED deltas in go-sdk-v1 matching the code |
| m3 | grpcx | status.go:42-45 | a Rule without Message sends the whole wrapped chain to the client | default to the sentinel's text (`target.Error()`) |
| m4 | lifecycle, healthx | lifecycle.go:212,225; healthx.go:39,63 | `Every` positional bool+logger; interval <= 0 panics (also `healthx.New(0)`) | options; validate intervals |
| m5 | pgx | pgx.go:1 | package name collides with jackc/pgx | rename to `postgres` |
| m6 | pgx | pgx.go:29-32 | `NewWithConfig` keeps Open's 5s ctx for background warm-up | `context.WithoutCancel` for NewWithConfig; ping with the timed ctx |
| m7 | pgx | pgx.go:55 | `OpenOrLocal` nil logger panics | default `slog.Default()` |
| m8 | grpcx | requestid.go:17 | incoming `x-request-id` not length/charset checked | accept <= 128 chars `[A-Za-z0-9._-]`, else generate |
| m9 | outbox | store.go:84-88 | empty `EventID` accepted then silently dropped by ON CONFLICT; no retention | reject empty EventID; `Purge(ctx, olderThan)` |
| m10 | consume | dedupe.go:26,37-39,70 | Dedupe validates per call, DDL helper not at all; `Seen` takes `pgx.Tx`, `Processed` an interface | validate in `NewDedupe` (returns error); accept an executor interface |
| m11 | consume | consume.go:21-27,179-186 | `Record` lacks Partition/Timestamp; `Headers map` drops duplicates; DLQ lacks source partition/offset/group | add fields, `Headers []Header`, DLQ headers |
| m12 | flags | flags.go:104,116 | WARN per failed evaluation; `Close` calls global `openfeature.Shutdown()` | rate-limit the log; shut down only this client's provider |
| m13 | kafkax | kafkax.go:68 | `Close` doc says it flushes; no Kafka health checker | fix doc; `kafkax.Checker(client)` |
| m14 | upstream | upstream.go:70-81 | named `Dial` but lazy; no round_robin default | rename `NewClient`; default `round_robin` service config |
| m15 | auth, idempotency | auth.go:33-37; idempotency.go:17-25 | `Principal.Type` plain string; ad-hoc errors | `type PrincipalType string`; `ErrInvalidKey` |
| m16 | module | go.mod | golangci-lint tool + testcontainers enlarge every consumer's module graph | evaluate a separate module for testkit/tooling; keep only if sync/vendoring keep working |
| m17 | testkit | fixtures.go:31-37; testkit.go:43-49 | with GO_SDK_REQUIRE_DOCKER only the first SharedPostgres caller fails; relies on Ryuk | Fatal in later callers; document Ryuk |
| m18 | lifecycle | lifecycle.go:96-111 | worst-case shutdown = drain delay + 2 x grace | one shared deadline (or document) |

Missing tests to add with the fixes: consumer rebalance/revoke, trace and request id on the consume side, relayer under broker outage and lease expiry, `WithTx` with a panicking fn, poison batch, `Load` with embedded groups, zero-value resilience options. Replace `time.Sleep` synchronisation in lifecycle_test.go:184,217,264,322 and consume_test.go:92,147.

## Re-review (2026-10-13, after packets 9–15)

All first-round findings fixed except where noted below. Owner decisions: park only non-retriable errors and add `Store.Requeue`; Kafka headers in kebab-case with one constant set in `kafkax`; every constructor that can be misconfigured returns `(T, error)`, no panics; `KAFKA_BROKERS` and `FLIPT_ADDR` required when `IsStrict()`.

| id | rank | where | problem | fix |
|---|---|---|---|---|
| N1 | blocker | outbox/relayer.go:129 | one 10s mark timeout for the whole batch while publishing may run ClaimLock/2; rows published after 10s are not marked and are re-published | per-mark timeout; test with a slow publisher and a large ClaimLock |
| N2 | blocker | outbox/store.go:78,107; README 131-143 | `ADD COLUMN seq BIGSERIAL` numbers existing rows in heap order (retried rows reorder) and rewrites the table under ACCESS EXCLUSIVE | stepwise migration: nullable `seq bigint`, backfill `row_number() OVER (ORDER BY created_at, event_id)`, sequence + setval + default, indexes `CONCURRENTLY`; doc "stop v0 relayers first"; SchemaSQL doc |
| N3 | major | outbox/store.go:161-184; relayer.go:97-99 | claim probes every blocked row; backlog of one aggregate drains quadratically | claim each aggregate's consecutive run in one pass (stop at first failure, release the rest); keep blocked aggregates out of the scan; expose blocked/parked count |
| N4 | major | consume.go:149; kafkax.go:105; consume.go:186-187 | unbounded fetch while rebalances are blocked; commit failure after losing the generation kills the consumer | `PollRecords` with `Options.MaxPollRecords` (default 500); lost-generation commit errors logged, not fatal |
| N5 | major | relayer.go:193-203; doc.go:14-16 | transient broker errors count toward parking; recovery doc leaves attempts | park only non-retriable errors; transient retry forever with capped backoff; `Store.Requeue(ctx, eventID)` resets attempts |
| N6 | major | consume.go:21-29; outbox/relayer.go:25-26 | mixed header naming, duplicated constants | kebab-case: `event-id`, `event-type`, `x-request-id`, `traceparent`, `dlq-error`, `dlq-source-topic`, `dlq-source-partition`, `dlq-source-offset`, `dlq-source-group`; one constant set in `kafkax` |
| N7 | minor | consume.go:106-111 | group-less client accepted by NewConsumer, first commit fails | reject in constructor |
| N8 | minor | lifecycle.go:106; grpcx.go:154 | Serve uses the whole grace, closers get an expired ctx | `Options.CloseGrace` reserve (components get grace minus reserve) |
| N9 | minor | relayer.go:165-166 | missing topic consumes attempts and parks rows | `NewRelayer` validates and returns an error; a row with no resolvable topic is a config error logged at ERROR, not an attempt |
| N10 | minor | postgres.go:76-83 | `runtime.Goexit` skips rollback; recover-path rollback has no timeout | unconditional deferred rollback with a timeout |
| N11 | minor | resilience.go:115-120 | zero `Policy` panics in `Do` | `Do` returns an error for a zero Policy |
| N12 | minor | delta spec.md:290 | wording says a fetch with a failed DLQ write is never committed; code commits records before it | fix spec wording |
| N13 | minor | consume.go:260 | request id from Kafka headers not validated | `reqctx.ValidRequestID`, used by grpcx and consume |
| N14 | minor | outbox.NewStore panics; NewRelayer/NewErrorTable don't validate; flags.NewWithProvider differs | constructors return `(T, error)`; `NewErrorTable` rejects `codes.OK` and nil targets |
| N15 | minor | store.go:108 | unused `claim_idx` | drop from SchemaSQL; migration note |
| N16 | minor | examples/hello config.go:8-9; template config.go.tmpl:8-9 | stale "Enabled field" comment | fix |
| N17 | minor | groups.go:92,119 | `KAFKA_BROKERS`, `FLIPT_ADDR` default to localhost | required when `IsStrict()` (Validate) |
