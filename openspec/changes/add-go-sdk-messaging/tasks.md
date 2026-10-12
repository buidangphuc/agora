All paths are under `platform-core/` unless they start with `openspec/`. SDK root = `packages/go-sdk`. Code track only: every scenario names its unit or integration test and gets a `not-testable` entry in `platform-core/FEATURES.yaml` (done in `## 6`).

Shared rules:
- Only `## 2` edits `go.mod`/`go.sum`; it adds every new module and blank-imports them in `internal/deps/deps.go`, which `## 5` deletes.
- Done check per packet: `make check` in the SDK root (Docker on; Redpanda via testcontainers `modules/redpanda`).
- Red → green.
- Never use a `Red-First-Exempt` trailer.
- Do not push.

## 1. Tooling + lifecycle.StopAfter (wave 1)

Owns: `tools/sync-go-sdk.sh`, `tools/sync-go-sdk_test.sh`, `make/go-service.mk`, `templates/go-service/Dockerfile`, `packages/go-sdk/pkg/lifecycle/**`, `docs/ADR/0016-go-service-convention-and-sdk.md` (tooling and lifecycle lines only).

- [x] 1.1 `TestStopAfterOrdersShutdown`, then implement `lifecycle.StopAfter(primary Component, dependents ...Component) Component`.
- [x] 1.2 Extend `sync-go-sdk_test.sh` red first (no `generated/` in the scratch copy, and assert local-prefixes), then fix the script, `make docker` and the template.

## 2. Config groups, kafkax, dependencies (wave 1)

Owns: `packages/go-sdk/go.mod`, `go.sum`, `internal/deps/deps.go`, `pkg/config/groups.go` + its tests (new groups only), `pkg/kafkax/**`, `pkg/testkit/**` (add a Redpanda fixture).

Contract:

```go
// pkg/config
type Kafka struct{ Brokers string `env:"KAFKA_BROKERS" default:"localhost:9092"` }
func (k Kafka) BrokerList() []string
type Outbox struct{ PollInterval time.Duration `env:"OUTBOX_POLL_INTERVAL" default:"1s"`; BatchSize int `env:"OUTBOX_BATCH_SIZE" default:"100"`; ClaimLockSeconds int `env:"OUTBOX_CLAIM_LOCK_SECONDS" default:"60"`; MaxAttempts int `env:"OUTBOX_MAX_ATTEMPTS" default:"10"` }
type Flags struct{ Enabled bool `env:"FEATURE_FLAGS_ENABLED" default:"true"`; FliptAddr string `env:"FLIPT_ADDR" default:"localhost:9000"`; EvalTimeoutMS int `env:"FEATURE_FLAGS_EVAL_TIMEOUT_MS" default:"500"` }

// pkg/kafkax
type Producer struct{ /* *kgo.Client */ }
func NewProducer(c config.Kafka, opts ...kgo.Opt) (*Producer, error)
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error
func (p *Producer) Close()
func NewConsumerClient(c config.Kafka, group string, topics ...string) (*kgo.Client, error) // DisableAutoCommit, kotel

// pkg/testkit
func Redpanda(t testing.TB) (brokers string)
```

- [ ] 2.1 Add the modules: franz-go `kgo` + `plugin/kotel`, `open-feature/go-sdk`, the Flipt OpenFeature provider used by team-order (match its import), and testcontainers `modules/redpanda`. Update `deps.go`.
- [ ] 2.2 Tests first: `TestPublishAddsTraceAndRequestHeaders`, `TestPublishReturnsErrorWhenBrokerDown`, and group default tests. Then implement.

## 3. outbox (wave 2)

Owns: `pkg/outbox/**`.

Contract:

```go
type Row struct{ EventID, AggregateType, AggregateID, EventType string; Payload []byte; RequestID string }
type Store struct{ /* pool, table */ }
func NewStore(pool *pgxpool.Pool, table string) *Store // table validated against ^[a-z_][a-z0-9_]*$
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, r Row) error
type Publisher interface{ Publish(ctx context.Context, topic string, key, value []byte) error }
type RelayerOptions struct{ Topic string; Config config.Outbox; BaseBackoff, MaxBackoff time.Duration; Logger *slog.Logger }
func NewRelayer(s *Store, p Publisher, o RelayerOptions) *Relayer
func (r *Relayer) Run(ctx context.Context) error
func (r *Relayer) RelayOnce(ctx context.Context) (Report, error)
type Report struct{ Claimed, Published, Retried, Parked int }
// SchemaSQL(table) returns the CREATE TABLE + indexes (family A DDL) for tests and new services.
func SchemaSQL(table string) string
```

Promote family A SQL (team-domain `internal/store/outbox_pg.go`, `internal/events/relayer.go`). Mark-published clears `error` and returns not-found on 0 rows.

- [ ] 3.1 Tests first (testkit Postgres): `TestConcurrentRelayersPublishOnce`, `TestFailingRowBacksOffThenParks`, `TestEnqueueRollsBackWithTx`. Then implement.

## 4. consume + dedupe (wave 2)

Owns: `pkg/consume/**`.

Contract:

```go
type Record struct{ Topic string; Key, Value []byte; Headers map[string]string; Offset int64 }
type Handler func(ctx context.Context, r Record) error
func Permanent(err error) error
type Options struct{ DLQTopic string; MaxAttempts int; BaseBackoff, MaxBackoff time.Duration; Logger *slog.Logger }
type DLQProducer interface{ Publish(ctx context.Context, topic string, key, value []byte) error } // kafkax.Producer satisfies it; DLQ headers are added via a ctx-free variant: define PublishRecord(ctx, *kgo.Record) on kafkax if needed and report
func NewConsumer(client *kgo.Client, h Handler, dlq DLQProducer, o Options) *Consumer
func (c *Consumer) Run(ctx context.Context) error
type Dedupe struct{ table string }
func NewDedupe(table string) Dedupe // default "processed_events"
func (d Dedupe) Seen(ctx context.Context, tx pgx.Tx, consumer, eventID string) (bool, error)
func DedupeSchemaSQL(table string) string // (consumer TEXT, event_id TEXT, processed_at TIMESTAMPTZ DEFAULT now(), PRIMARY KEY (consumer, event_id))
```

- [ ] 4.1 Tests first (Redpanda + Postgres): `TestPermanentErrorGoesToDLQ`, `TestDLQFailureDoesNotCommit`, `TestDedupeIsPerConsumer`, `TestDedupeRollsBackWithTx`. Then implement.

## 5. flags + upstream, cleanup (wave 2)

Owns: `pkg/flags/**`, `pkg/upstream/**`, deletion of `internal/deps/deps.go` (+ `go mod tidy`; may touch `go.mod`/`go.sum` for the tidy only, merged last).

Contract:

```go
// pkg/flags
type Evaluator interface{ BooleanEnabled(ctx context.Context, flag string, def bool) bool }
func New(ctx context.Context, c config.Flags, logger *slog.Logger) (*Client, error) // Client implements Evaluator + Close(ctx)
func Disabled() Evaluator

// pkg/upstream
type Option func(*options)
func AsService(id string, scopes ...string) Option
func WithTimeout(d time.Duration) Option // default per-call bound
func Dial(addr string, opts ...Option) (*grpc.ClientConn, error)
func Bounded(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) // 500ms margin before parent deadline
```

- [ ] 5.1 Tests first: `TestUnreachableFliptReturnsDefault`, `TestAsServiceReplacesPrincipal`, `TestForwardsRequestIDAndPrincipal`, `TestBoundedRespectsParentDeadline`. Then implement, promoting team-order `internal/featureflags` and the order/engagement upstream interceptors.
- [ ] 5.2 Delete `internal/deps`, `go mod tidy`, `make check` green.

## 6. Close (orchestrator)

- [ ] 6.1 Add FEATURES entries; run `ci_local.sh`; tag `go-sdk/v0.2.0` locally (do not push).
