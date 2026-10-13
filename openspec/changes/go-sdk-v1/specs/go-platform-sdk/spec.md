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

### Requirement: Error rules can fix the client message

`grpcx.ErrorTable` SHALL map sentinel errors to `grpcx.Rule{Code codes.Code; Message string}`. A matching rule with a Message SHALL use it as the status message; an empty Message SHALL keep `err.Error()`. `grpcx.Code(c)` SHALL return `Rule{Code: c}`. Unmatched errors keep becoming Internal with the fixed internal message.

#### Scenario: Rule message replaces wrapped error text
- **WHEN** `fmt.Errorf("listing 7: %w", ErrNotFound)` matches `Rule{NotFound, "not_found"}`
- **THEN** the status is NotFound with message `not_found`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusUsesRuleMessage`

#### Scenario: A rule without message does not change today's text
- **WHEN** the same error matches `grpcx.Code(codes.NotFound)`
- **THEN** the message is `listing 7: not_found`
- **VERIFIED BY**: `TestStatusKeepsErrorTextWithoutMessage`

### Requirement: Local-only database fallback

`pgx.OpenOrLocal(ctx, c config.Postgres, rt config.Runtime, logger) (*pgxpool.Pool, error)` SHALL return the pool when the database is reachable; when it is not, SHALL return `(nil, nil)` and log a warning only if `rt.IsLocal()`, and return the error otherwise.

#### Scenario: Unreachable database outside local does not fall back
- **WHEN** the database is unreachable with `ENV=dev` and with `ENV=local`
- **THEN** `dev` returns an error and `local` returns a nil pool and nil error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/pgx` `TestOpenOrLocalFallsBackOnlyLocally`

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

`lifecycle.Every(interval time.Duration, runAtStart bool, fn func(context.Context) error, logger *slog.Logger) Component` SHALL run fn on each tick, log a returned error and keep running, and stop when ctx ends. `lifecycle.HTTP(srv *http.Server, grace time.Duration) Component` SHALL serve until ctx ends and then call `Shutdown` bounded by grace.

#### Scenario: A failing tick does not stop the worker
- **WHEN** fn fails on its first call
- **THEN** it is called again on the next tick, and Run returns nil after ctx is cancelled
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/lifecycle` `TestEveryKeepsRunningAfterError`

#### Scenario: HTTP server drains in-flight requests
- **WHEN** ctx is cancelled while a request is in flight
- **THEN** the request completes and Run returns nil within grace
- **VERIFIED BY**: `TestHTTPShutsDownGracefully`

### Requirement: Flags degrade on setup failure

`flags.NewOrDisabled(ctx, c, logger) (Evaluator, func(context.Context) error)` SHALL return the Flipt-backed evaluator, or `Disabled()` with a warning when setup fails; the closer is always safe to call.

#### Scenario: Setup failure does not stop the service
- **WHEN** flags are enabled with an empty `FLIPT_ADDR`
- **THEN** the evaluator returns caller defaults and the closer returns nil
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/flags` `TestNewOrDisabledDegrades`

### Requirement: Batch consumer

`consume.NewBatchConsumer(client, h BatchHandler, dlq DLQProducer, o BatchOptions)` with `BatchHandler func(ctx, []Record) error` SHALL hand each fetched batch to h; on nil it SHALL commit; on an error wrapped with `Permanent` it SHALL send every record of the batch to the DLQ with the same headers as the per-record consumer and then commit; on any other error it SHALL retry the same batch after backoff and SHALL NOT commit.

#### Scenario: Transient batch failure is retried without commit
- **WHEN** h fails transiently once and then succeeds
- **THEN** h sees the same records twice and offsets are committed only after the success
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestBatchRetriesWithoutCommit`

#### Scenario: Permanent batch failure goes to the DLQ
- **WHEN** h returns `Permanent(err)`
- **THEN** every record is on the DLQ with `error` and `source-topic` headers and the batch is committed
- **VERIFIED BY**: `TestBatchPermanentGoesToDLQ`

### Requirement: Dedupe can be checked without writing

`consume.Dedupe.Processed(ctx, q, consumer, eventID)` SHALL report whether `(consumer, eventID)` exists, without inserting, and SHALL return database errors rather than false.

#### Scenario: Processed reports existing pairs only
- **WHEN** `(a, e1)` was marked and `(b, e1)` was not
- **THEN** `Processed(a, e1)` is true, `Processed(b, e1)` is false, and no row is inserted by either call
- **VERIFIED BY**: `TestDedupeProcessedIsReadOnly`

#### Scenario: Database errors are not reported as not processed
- **WHEN** the dedupe table does not exist
- **THEN** `Processed` returns a non-nil error
- **VERIFIED BY**: `TestDedupeProcessedReturnsErrors`

### Requirement: Consumer clients can read without a group

`kafkax.NewConsumerClient` with an empty group SHALL return a client that consumes the topics from the start, with no consumer group and no commits.

#### Scenario: Group-less client reads existing records and does not commit
- **WHEN** records exist on a topic and a group-less client is built for it
- **THEN** it reads them from the start, and no consumer group appears on the broker
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestGroupLessConsumerClient`

### Requirement: Upstream dial options and retry predicates

`upstream.WithDialOptions(opts ...grpc.DialOption) Option` SHALL append dial options after the SDK defaults. `resilience.RetryOptions.RetryIf func(error) bool` SHALL decide retries when set; nil keeps `Retryable`.

#### Scenario: Extra dial options are applied
- **WHEN** `Dial` gets `WithDialOptions` carrying a unary interceptor
- **THEN** the interceptor sees each call
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/upstream` `TestDialOptionsApplied`

#### Scenario: RetryIf narrows retries
- **WHEN** `RetryIf` admits only Unavailable and fn returns DeadlineExceeded under 3 attempts
- **THEN** fn runs once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/resilience` `TestRetryIfOverridesDefault`

### Requirement: Shared test fixtures

`testkit` SHALL export `FreePort(t) int`, `Eventually(t, timeout, fn func() bool)`, `Recorder` (implements `outbox.Publisher` and `consume.DLQProducer`, records topic/key/value/headers, `FailOn(key)`), `SharedPostgres(t) string` (one container per test binary) with `Schema(t, dsn) string` (a DSN whose `search_path` is a fresh schema dropped at cleanup), and `Produce(t, brokers, topic, key, value)` / `ReadAll(t, brokers, topic, n, timeout) []*kgo.Record`.

#### Scenario: Schemas isolate tests that share a container
- **WHEN** two tests create the same table through `Schema` DSNs on one `SharedPostgres`
- **THEN** neither sees the other's rows, and the schemas are gone after cleanup
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/testkit` `TestSchemaIsolation`

#### Scenario: Recorder can fail one key and not others
- **WHEN** `FailOn("k1")` is set and keys k1 and k2 are published
- **THEN** k1 returns an error and only k2 is recorded
- **VERIFIED BY**: `TestRecorderFailOn`

### Requirement: Sync keeps the go-sdk requirement

`sync-go-sdk.sh` SHALL ensure the service `go.mod` requires the go-sdk module at the placeholder version used with the `replace` before `go mod tidy`, without adding files to a service that already imports the SDK.

#### Scenario: Fresh sync leaves a require line without failing tidy
- **WHEN** a scratch service with no SDK imports is synced
- **THEN** `go.mod` has the go-sdk `require` and `replace`, and adding one SDK import builds without editing `go.mod`
- **VERIFIED BY**: `platform-core/tools/sync-go-sdk_test.sh`
