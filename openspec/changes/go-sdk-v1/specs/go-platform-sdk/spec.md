## ADDED Requirements

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

`grpcx.ErrorTable` SHALL be an ordered list built with `grpcx.NewErrorTable(rules ...grpcx.Rule)`, where `Rule{Target error; Code codes.Code; Message string}`. `grpcx.Status` SHALL use the first rule whose Target matches with `errors.Is`, so the order is the precedence. A matching rule with a Message SHALL use it as the status message; an empty Message SHALL send `Target.Error()`, never the wrapped chain. `context.Canceled` and `context.DeadlineExceeded` SHALL map to `Canceled` and `DeadlineExceeded` and be logged at Info. Unmatched errors keep becoming Internal with the fixed internal message.

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

`flags.NewOrDisabled(ctx, c, logger) (Evaluator, func(context.Context) error)` SHALL return the Flipt-backed evaluator, or `Disabled()` with a warning when setup fails; the closer is always safe to call. `Evaluator.BooleanEnabled(ctx, flag, def, opts ...EvalOption) bool` SHALL use the id of the principal in ctx as the targeting key (`anonymous` without one) unless `WithTargetingKey` overrides it, SHALL add `WithAttributes` to the evaluation context, and SHALL return def on any error or timeout. `Close` SHALL shut down only the client's own provider, not other clients.

#### Scenario: Setup failure does not stop the service
- **WHEN** flags are enabled with an empty `FLIPT_ADDR`
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

`outbox.Publisher` SHALL be `PublishRecord(ctx, *kgo.Record) error`. The relayer SHALL publish every row as a record keyed by `AggregateID`, on the row's `Topic` or else the relayer's `Topic`, with headers `event_id`, `event_type`, `x-request-id` and the `traceparent` stored at enqueue. Order SHALL hold per aggregate: a row SHALL NOT be claimed while an older unpublished row of the same aggregate exists, whether it is backing off, leased or parked; other aggregates SHALL NOT be held back. One relay batch SHALL be bounded by half of `Outbox.ClaimLock` (`OUTBOX_CLAIM_LOCK`). `EnqueueTx` SHALL reject an empty `EventID` with `ErrEmptyEventID`, and `Store.Purge(ctx, olderThan)` SHALL delete only published rows older than the duration. `SchemaSQL` SHALL add the `topic`, `traceparent` and `seq` columns and the two supporting indexes to a table created by an older SDK.

#### Scenario: Rows of one aggregate publish in enqueue order
- **WHEN** several rows of one aggregate are enqueued, some in the same transaction
- **THEN** they are published in enqueue order
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/outbox` `TestOrderPerAggregate`

#### Scenario: A failing row blocks its own aggregate only
- **WHEN** an older row of aggregate A fails and rows of A and B are pending
- **THEN** B's rows are published and A's newer rows wait
- **VERIFIED BY**: `TestFailingRowBlocksItsAggregateOnly`

#### Scenario: A parked row keeps blocking its aggregate
- **WHEN** a row of aggregate A is parked as failed
- **THEN** newer rows of A are not claimed
- **VERIFIED BY**: `TestParkedRowBlocksItsAggregate`

#### Scenario: Published records carry the headers
- **WHEN** a row is enqueued in a context with a request id and a trace
- **THEN** the published record has `event_id`, `event_type`, `x-request-id` and `traceparent` headers
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

`consume.NewConsumer(client, h, dlq, o) (*Consumer, error)` and `NewBatchConsumer` SHALL return an error for a nil client, handler or DLQ producer and for an empty `DLQTopic`. The handler context SHALL carry the W3C trace context and `x-request-id` of the record; a batch handler context SHALL carry a span linked to each record's span. Offsets SHALL be committed once per fetch, after every record of it is handled or dead-lettered, and never for a fetch whose dead-lettering failed. `consume.Record` SHALL carry `Partition`, `Timestamp` and `Headers []Header`, which keeps duplicate header keys.

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

### Requirement: Sync keeps the go-sdk requirement

`sync-go-sdk.sh` SHALL ensure the service `go.mod` requires the go-sdk module at the placeholder version used with the `replace` before `go mod tidy`, without adding files to a service that already imports the SDK.

#### Scenario: Fresh sync leaves a require line without failing tidy
- **WHEN** a scratch service with no SDK imports is synced
- **THEN** `go.mod` has the go-sdk `require` and `replace`, and adding one SDK import builds without editing `go.mod`
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`

## MODIFIED Requirements

### Requirement: Settings load from environment with the services' existing tags

`config.Load[T]()` SHALL populate a settings struct from environment variables. It reads `env:"NAME"` and `default:"value"` tags, the tag names the services use today, and recurses into nested structs without prefixing. It SHALL return an error naming every variable that fails to parse or is marked `required` and unset. After parsing, `Load` SHALL call `Validate() error` on `T` when it has one, and on every nested or embedded group that has one, and SHALL return all their errors joined. `config.Postgres.Validate()` SHALL reject an empty `URL`: the database has no enable flag. `config.Runtime.Validate()` SHALL reject an empty `ENV`. The SDK SHALL provide the embeddable groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox` and `Flags`. Their variable names and defaults SHALL be `ENV` (required, no default), `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT` (required), `SHUTDOWN_GRACE` (`10s`), `SHUTDOWN_DRAIN_DELAY` (`0s`), `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `KAFKA_BROKERS`, `OUTBOX_POLL_INTERVAL` (`1s`), `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK` (`60s`), `OUTBOX_MAX_ATTEMPTS`, `FEATURE_FLAGS_ENABLED`, `FLIPT_ADDR` and `FEATURE_FLAGS_EVAL_TIMEOUT` (`500ms`). Every duration SHALL be a `time.Duration` field read as a Go duration, and no key carries a unit suffix. There is no `GRPC_REFLECTION_ENABLED` and no `DATABASE_ENABLED`. `Runtime.IsLocal()` SHALL be true only for an explicit `ENV` of `local` or `test`.

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

`lifecycle.Run(ctx, opts, components...)` SHALL start every component's `Run(ctx) error` in an errgroup. It SHALL return when ctx is cancelled or any component returns. On return it SHALL first call the `OnShutdown` hooks (for example health drain), then cancel the components, wait for them, then call the closers in reverse registration order; components and closers SHALL share one `ShutdownGrace` budget, so the worst case after the drain delay is one grace period. It SHALL return the first non-context error. `Options.DrainDelay` (default 0, from `SHUTDOWN_DRAIN_DELAY` via `config.GRPC.DrainDelay`) SHALL be waited after the hooks and before cancelling components, so probes observe NOT_SERVING. `lifecycle.Stack` SHALL collect closers during wiring: `Push(f)` registers, `Close(ctx)` runs them once in reverse order and joins errors, so a composition root can release what it opened when a later step fails.

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

`postgres.Open(ctx, cfg config.Postgres)` SHALL parse the URL, apply `MaxConns`, create the pool and ping it within 5s. It SHALL return an error if the ping fails. `postgres.Checker(pool)` SHALL return a health check that pings. `postgres.WithTx(ctx, pool, fn)` SHALL commit when `fn` returns nil and roll back when it returns an error or panics, without leaking the connection.

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

### Requirement: Retries and circuit breaking use shared policies

`resilience` SHALL expose `Retry(opts) (Policy, error)` and `Breaker(opts) (Policy, error)` built on failsafe-go behind an opaque `Policy` type, plus `Do(ctx, fn, policies...)`. Zero-value options SHALL take defaults (3 attempts, 100ms to 5s backoff, breaker threshold 5, open delay 30s) and negative values SHALL be rejected with an error. Retries SHALL use exponential backoff with jitter. `Retryable(err)` SHALL be an allow-list: only gRPC `Unavailable`, `ResourceExhausted`, `Aborted` and `DeadlineExceeded` are retried; plain errors, `Internal` and context cancellation are not.

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
