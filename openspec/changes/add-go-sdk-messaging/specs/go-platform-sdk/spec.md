## ADDED Requirements

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

### Requirement: Sync prepares a buildable, correctly linted service

`sync-go-sdk.sh` SHALL run `buf generate` in the service when `generated/` is missing and `buf.gen.yaml` exists. It SHALL rewrite goimports `local-prefixes` in the synced `.golangci.yml` to the service's module path. `make docker` SHALL pass `GRPC_PORT` as a build argument, and the template Dockerfile SHALL have no `GRPC_PORT` default.

#### Scenario: Sync works on a fresh checkout without generated code
- **WHEN** the script runs on a scratch service with no `generated/`
- **THEN** it generates the stubs, the service builds, and `.golangci.yml` names the service module as local prefix
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`
