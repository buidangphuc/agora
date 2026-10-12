All paths below are relative to `platform-core/` unless they start with `openspec/`. SDK root = `packages/go-sdk` (module `github.com/buidangphuc/platform-core/packages/go-sdk`, `go 1.26.0`). E2E track: none — no user-facing behaviour; every scenario names its unit test in a `VERIFIED BY` line.

Shared rules for every packet:
- Only `## 1` edits `packages/go-sdk/go.mod` / `go.sum`. Later packets must not change them. If a needed module is missing, stop and report it instead of adding it.
- Local check in the SDK root: `go vet ./... && go test -race ./...` plus `go tool golangci-lint run ./...` (config `packages/go-sdk/.golangci.yml`). Docker is available for testcontainers.
- Package docs: one `doc.go` per package, saying what the package is for. Comments only where the code is not obvious.
- No global mutable state, except `otelx` setting the OTel globals, which is the OTel contract.

Shared contract — `pkg/reqctx` (written in `## 1`, used by everyone):
```go
package reqctx
const MetadataRequestID = "x-request-id"
func WithRequestID(ctx context.Context, id string) context.Context
func RequestID(ctx context.Context) string      // "" when absent
func WithPrincipalID(ctx context.Context, id string) context.Context
func PrincipalID(ctx context.Context) string    // "" when absent
```

## 1. Foundation: module, reqctx, lint config (wave 1)

Owns: `packages/go-sdk/go.mod`, `packages/go-sdk/go.sum`, `packages/go-sdk/internal/deps/deps.go`, `packages/go-sdk/pkg/reqctx/**`, `packages/go-sdk/.golangci.yml`, `packages/go-sdk/Makefile`; deletes `packages/go-sdk/pkg/auth/**`, `packages/go-sdk/pkg/config/**`, `packages/go-sdk/pkg/resilience/**` (rewritten in wave 2).

- [x] 1.1 Write `pkg/reqctx/reqctx_test.go` (round-trip and empty-default cases), see it fail, then implement the contract above; verify `TestRequestIDRoundTrip`, `TestPrincipalIDRoundTrip`, `TestEmptyWhenAbsent`.
- [x] 1.2 Bump `go 1.26.0`. Require current releases of:
  - `github.com/caarlos0/env/v11`
  - `github.com/grpc-ecosystem/go-grpc-middleware/v2`
  - `golang.org/x/sync`
  - `google.golang.org/grpc`
  - `go.opentelemetry.io/otel`, its `sdk`, `sdk/metric`, `exporters/otlp/otlptrace/otlptracegrpc` and `exporters/otlp/otlpmetric/otlpmetricgrpc`
  - `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc`
  - `github.com/jackc/pgx/v5`
  - `github.com/golang-migrate/migrate/v4` (with the `database/pgx/v5` and `source/iofs` drivers)
  - `github.com/failsafe-go/failsafe-go`
  - `github.com/testcontainers/testcontainers-go` and its `modules/postgres`
  - `github.com/google/uuid`

  `internal/deps/deps.go` blank-imports each so `go mod tidy` keeps them (removed in `## 9`). Add `tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint`. Verify with `go mod tidy && go build ./...`.
- [x] 1.3 `.golangci.yml` (golangci-lint v2 format) enables govet, staticcheck, errcheck, revive, gosec, contextcheck, sloglint, errorlint, and gofumpt + goimports formatters. `Makefile` targets: `test` (race), `lint`, `check` (vet + lint + test). Verify `make check` is green.

## 2. Tooling, ADR, docs (wave 1)

Owns: `tools/sync-go-sdk.sh`, `tools/sync-go-sdk_test.sh`, `make/go-service.mk`, `templates/go-service/Dockerfile`, `docs/ADR/0016-go-service-convention-and-sdk.md`, `packages/go-sdk/README.md`.

- [x] 2.1 Write `tools/sync-go-sdk_test.sh` first; see it fail because the script is missing. The test:
  - makes a throwaway git clone in a temp dir and creates a local tag `go-sdk/v0.0.0-test` there;
  - makes a scratch copy of `team-sharing`;
  - runs the script and asserts the "Sync vendors the tagged tree" scenario (`VERSION` names the tag, the `replace` line exists, `go build ./...` passes);
  - runs again with tag `go-sdk/v9.9.9-missing` and asserts a non-zero exit and an unchanged tree (compare `find | sort | md5`).
  
  Then implement `tools/sync-go-sdk.sh <tag> <service-dir>...` per the spec. It uses `git archive <tag>:platform-core/packages/go-sdk` into a temp dir and swaps it in only after success, so a failed run changes nothing. It also copies `make/go-service.mk` and `packages/go-sdk/.golangci.yml`, then runs `go mod edit -replace` and `go mod tidy`. Verify `bash tools/sync-go-sdk_test.sh`.
- [x] 2.2 `make/go-service.mk`. The including Makefile sets `SERVICE`, `GRPC_PORT` and optionally `BINARIES` (default `server`). Targets:
  - `help`
  - `proto` (`buf generate`)
  - `build`
  - `run` (`go run ./cmd/$(firstword $(BINARIES))`)
  - `test` (`go test -race ./...`)
  - `lint` (`go tool golangci-lint run ./...`)
  - `check-env` (`go test ./internal/config -run TestEnvExampleInSync`)
  - `check` (vet + lint + test + check-env)
  - `migrate` (`go run ./cmd/$(firstword $(BINARIES)) migrate up` — documented contract for phase 2)
  - `docker` (`docker build -t $(SERVICE):dev .`)
  
  Verify with `make -f make/go-service.mk help` from a scratch dir.
- [x] 2.3 `templates/go-service/Dockerfile`: `golang:1.26` build stage with `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`, and an `ARG BIN=server`, then `gcr.io/distroless/static:nonroot` running `USER nonroot:nonroot` with `EXPOSE ${GRPC_PORT}`. Copy `go.mod go.sum third_party/` before `go mod download` so the vendored SDK resolves. Verify by building the scratch team-sharing from 2.1.
- [x] 2.4 ADR-0016, following ADR-0001's format. It covers:
  - Context: the findings in `proposal.md`.
  - Decision:
    - the layout tree;
    - explicit wiring;
    - the `Run(ctx) error` + lifecycle rule;
    - errors mapped in transport;
    - the library list with one reason each;
    - patterns borrowed from kratos (lifecycle, error model) and gofr (per-dependency health), plus why there is no framework, no DI container and no reflection;
    - distribution (tags `go-sdk/vX.Y.Z`, vendored `third_party/go-sdk`, `replace`, VERSION);
    - AI concerns out of scope.
  - Open question: task store River vs RabbitMQ (ADR-0002), decided in phase 4.
  - Consequences.
  
  `packages/go-sdk/README.md`: a package index (one line each) and "how a service adopts it" (sync script, then copy `examples/hello`), linking the ADR. Verify with a docs read-through: every package in `## 3`–`## 9` is listed.

## 3. config (wave 2)

Owns: `packages/go-sdk/pkg/config/**`.

Contract:
```go
func Load[T any]() (T, error)                 // caarlos0/env ParseAs; Options{DefaultValueTagName:"default", RequiredIfNoDef:false}; errors joined, each naming its var
func DeclaredEnvKeys(v any) []string          // sorted, from env field params (nested structs included)
func CheckEnvExample(path string, v any) error
type Runtime struct { Env string `env:"ENV" default:"local"`; LogLevel string `env:"LOG_LEVEL" default:"info"`; LogJSON bool `env:"LOG_JSON" default:"true"` }
func (r Runtime) IsLocal() bool               // Env == "local" || Env == "test"
type GRPC struct { Host string `env:"GRPC_HOST" default:"0.0.0.0"`; Port int `env:"GRPC_PORT,required"`; ReflectionEnabled bool `env:"GRPC_REFLECTION_ENABLED" default:"true"`; ShutdownGraceSeconds float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"` }
func (g GRPC) Addr() string                   // net.JoinHostPort
func (g GRPC) ShutdownGrace() time.Duration
type Postgres struct { Enabled bool `env:"DATABASE_ENABLED" default:"true"`; URL string `env:"DATABASE_URL"`; MaxConns int32 `env:"DB_MAX_CONNS" default:"10"` }
type OTel struct { Enabled bool `env:"OTEL_ENABLED" default:"false"`; Endpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`; ServiceName string `env:"OTEL_SERVICE_NAME"` }
```
- [x] 3.1 Tests first: `TestLoadDefaultsAndOverrides`, `TestLoadReportsBadVariable`, `TestCheckEnvExampleReportsBothDirections`, `TestDeclaredEnvKeysIncludesNested` (use `t.Setenv`; port the `.env.example` parsing rules from `team-order/internal/config/envcheck.go`). Then implement. Verify green.

## 4. logx + otelx (wave 2)

Owns: `packages/go-sdk/pkg/logx/**`, `packages/go-sdk/pkg/otelx/**`.

Contract:
```go
package logx
func New(r config.Runtime, service string) *slog.Logger   // JSON|text, level, attr service; handler wraps to add request_id, principal_id (reqctx), trace_id (otel span ctx) when present
func NewHandler(inner slog.Handler) slog.Handler           // the wrapping handler, exported for tests/custom sinks
package otelx
func Init(ctx context.Context, c config.OTel, service string) (shutdown func(context.Context) error, err error)
```
Write `pkg/config` usage against the `## 3` contract. If `## 3` has not merged, define the needed struct locally in the test only. Do not create `pkg/config`.
- [x] 4.1 Tests first: `TestHandlerAddsContextIDs`, `TestHandlerOmitsMissingIDs`, `TestNewHonoursLevelAndFormat`; then implement logx.
- [x] 4.2 Tests first: `TestInitDisabledIsNoop`, `TestInitEnabledUsesServiceFallback` (use an in-memory span exporter via an internal option or check the resource through the returned provider). Then implement otelx as the promotion of `team-domain/internal/observability/tracer.go` plus a meter provider. Verify green.

## 5. auth (wave 2)

Owns: `packages/go-sdk/pkg/auth/**`.

Contract:
```go
type Principal struct { ID, Type string; Scopes []string }
func (p Principal) Anonymous() bool                 // ID=="" || ID=="anonymous" || Type=="anonymous"
func (p Principal) HasScope(s string) bool          // exact match only
func FromContext(ctx context.Context) (Principal, bool)
func WithPrincipal(ctx context.Context, p Principal) context.Context   // also reqctx.WithPrincipalID
func UnaryServerInterceptor() grpc.UnaryServerInterceptor
func StreamServerInterceptor() grpc.StreamServerInterceptor
func RequirePrincipal(ctx context.Context) (Principal, error)          // codes.Unauthenticated
func RequireScopes(ctx context.Context, scopes ...string) (Principal, error) // Unauthenticated if no principal, PermissionDenied if a scope is missing
const MDPrincipalID, MDPrincipalType, MDPrincipalScopes = "x-principal-id", "x-principal-type", "x-principal-scopes"
```
- [x] 5.1 Tests first: `TestRequirePrincipalRejectsAnonymous`, `TestRequireScopesExactMatchNoAdminBypass`, `TestStreamInterceptorInjectsPrincipal`, `TestScopesParsingTrimsAndDropsEmpty`. Then implement, merging the helper set from `team-promotion/internal/interceptor/auth.go` with the stream interceptor from `team-engagement`. Service-specific helpers (RequireAdmin/Seller) stay out. Verify green.

## 6. healthx + lifecycle (wave 2)

Owns: `packages/go-sdk/pkg/healthx/**`, `packages/go-sdk/pkg/lifecycle/**`.

Contract:
```go
package healthx
type Check func(ctx context.Context) error
type Health struct{ /* wraps *health.Server */ }
func New(interval time.Duration, logger *slog.Logger) *Health  // starts SERVING (no checks yet)
func (h *Health) Register(name string, c Check)
func (h *Health) Server() *health.Server                        // for grpc registration
func (h *Health) Run(ctx context.Context) error                 // lifecycle component; returns nil on ctx done
func (h *Health) Drain()
package lifecycle
type Component interface{ Run(ctx context.Context) error }
type Func func(ctx context.Context) error                       // implements Component
type Options struct { ShutdownGrace time.Duration; OnShutdown []func(); Closers []func(context.Context) error; Logger *slog.Logger }
func Run(ctx context.Context, o Options, cs ...Component) error
```
- [x] 6.1 Tests first: `TestCheckerFailureFlipsStatus`, `TestDrainIsSticky`, `TestHangingCheckerFailsWithoutBlocking`; then implement healthx, promoting `team-domain/internal/bootstrap/lifecycle.go:198`.
- [x] 6.2 Tests first: `TestFailingComponentCancelsOthers`, `TestShutdownOrder`, `TestShutdownDeadline`; then implement lifecycle (errgroup + context). Verify green with `-race`.

## 7. pgx + migrate + testkit (wave 2)

Owns: `packages/go-sdk/pkg/pgx/**`, `packages/go-sdk/pkg/migrate/**`, `packages/go-sdk/pkg/testkit/**`.

Contract:
```go
package pgx
func Open(ctx context.Context, c config.Postgres) (*pgxpool.Pool, error)   // 5s ping deadline
func Checker(pool *pgxpool.Pool) healthx.Check                           // define as func(context.Context) error to avoid importing healthx
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error
package migrate
func Up(ctx context.Context, databaseURL string, fsys fs.FS) error         // ErrNoChange == nil
package testkit
func Postgres(t testing.TB) (url string)   // testcontainers postgres:17-alpine; t.Cleanup terminates; t.Skip if Docker is unavailable and GO_SDK_REQUIRE_DOCKER is unset
```
`Checker` returns `func(context.Context) error` (assignable to `healthx.Check`) so `pgx` does not import `healthx`. Use `config.Postgres` per the `## 3` contract. If `## 3` has not merged, declare a test-local struct and leave a note.
- [ ] 7.1 Tests first: `TestOpenFailsOnUnreachableDB`, `TestWithTxRollsBackOnError`, `TestWithTxCommits`, `TestUpIsIdempotent`, `TestUpFailsOnInvalidSQL`, `TestPostgresFixtureIsReachable`; then implement. Verify green with Docker running.

## 8. resilience (wave 2)

Owns: `packages/go-sdk/pkg/resilience/**`.

Contract:
```go
type RetryOptions struct { MaxAttempts int; BaseDelay, MaxDelay time.Duration }
type BreakerOptions struct { FailureThreshold uint; Delay time.Duration }
func Retry(o RetryOptions) failsafe.Policy[any]
func Breaker(o BreakerOptions) failsafe.Policy[any]
func Do(ctx context.Context, fn func(ctx context.Context) error, policies ...failsafe.Policy[any]) error
func Retryable(err error) bool    // false for the non-retryable gRPC codes in the spec and for context.Canceled
```
- [x] 8.1 Tests first: `TestRetrySkipsNonRetryableCodes`, `TestRetryRetriesUnavailable`, `TestBreakerOpensAfterFailures`; then implement. Verify green.

## 9. grpcx + example service (wave 3, after 1–8 merge)

Owns: `packages/go-sdk/pkg/grpcx/**`, `packages/go-sdk/examples/hello/**`; deletes `packages/go-sdk/internal/deps/deps.go` (then `go mod tidy`; this packet is allowed to touch `go.mod`/`go.sum` for that tidy only).

Contract:
```go
type ServerOptions struct { Config config.GRPC; Logger *slog.Logger; Health *healthx.Health; Extra []grpc.ServerOption }
func NewServer(o ServerOptions) *grpc.Server       // stats otelgrpc; chain recovery -> requestID -> logging(go-grpc-middleware v2 slog adapter) -> auth; health registered; reflection per config
func Serve(srv *grpc.Server, c config.GRPC) lifecycle.Component  // listen c.Addr(); GracefulStop on ctx done; Stop after c.ShutdownGrace()
type ErrorTable map[error]codes.Code
func Status(ctx context.Context, logger *slog.Logger, err error, t ErrorTable) error  // errors.Is match -> status with err.Error(); unmatched -> log + Internal "internal error"; nil -> nil
```
The example is a `HelloService` using `grpc_health_v1`-style hand-written service registration (no proto codegen), or `grpc.ServiceDesc` built by hand. It has one RPC `Greet` (requires a principal) and one RPC `Boom` (panics). Layout follows ADR-0016: `cmd/hello/main.go` < 20 lines, `internal/app/app.go` composition root, `internal/config`, `internal/grpc`.
- [ ] 9.1 Tests first: `TestRecoveryKeepsServing`, `TestRequestIDPropagates`, `TestStatusHidesUnknownErrors`, `TestStatusMapsKnownErrors` (bufconn); then implement grpcx.
- [ ] 9.2 Tests first: `TestShutdownDrainsHealthFirst`, `TestGreetRejectsMissingPrincipal` (run app on a free port, watch health, cancel ctx, assert NOT_SERVING is observed before the connection closes); then build the example. Delete `internal/deps`, run `go mod tidy`, and run `make check` green for the whole SDK.

## 10. Close (orchestrator)

- [ ] 10.1 All packets merged; `make -C platform-core/packages/go-sdk check` green; `bash platform-core/tools/sync-go-sdk_test.sh` green; `openspec validate add-go-platform-sdk-core --strict` passes.
- [ ] 10.2 Convention dry-run: a fresh agent deletes `examples/hello` in a throwaway worktree, rebuilds it from ADR-0016 + README alone and reports every guess. Each guess becomes a docs or code fix.
