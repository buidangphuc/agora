# ADR-0016 — Go service convention and SDK distribution

**Status:** Accepted · **Date:** 2026-10-12 · **Relates to:** ADR-0001, ADR-0002, ADR-0004, ADR-0010

## Context

The 15 Go services re-implement the platform layer (config, logging, tracing, auth, health, lifecycle, Postgres) in three drifting generations. Nothing enforces a shared shape. Findings from the code:

- The config binder and the `.env.example` check are copied into 7–9 repos (`team-order/internal/config/envcheck.go`, `team-domain/internal/config/envcheck.go`). The OTLP tracer is copied into 6 repos (`team-domain/internal/observability/tracer.go:22`).
- No service recovers from a handler panic (`team-order/internal/grpcserver/server.go:24`). One nil dereference kills the process.
- `OTEL_*` settings are dead in team-order, payment and promotion. They install the `otelgrpc` stats handler, but no tracer provider is initialised (`team-order/internal/interceptor/tracing.go:19`).
- Health is always SERVING in most repos (`team-order/internal/bootstrap/resources.go:28`). Only domain, identity and engagement probe the database (`team-domain/internal/bootstrap/lifecycle.go:198`).
- `team-sharing/cmd/server/main.go:32` treats `pgxpool.New` as a connection check. It is lazy, so the in-memory fallback never triggers.
- `platform-core/packages/go-sdk` exists but no service imports it. Its `auth.HasScope` lets the `admin` scope pass every check (`pkg/auth/auth.go:27`), which contradicts team-order's rule that bare `admin` does not open orders.

## Decision

### Layout

```
cmd/<binary>/main.go     # signals, config.Load, app.Run(ctx, settings), exit code
internal/app/            # composition root; has `const service = "<name>"`
internal/config/         # Settings struct, Validate, Load; its test checks ../../.env.example
internal/rpc/            # transport (package rpc): handlers, error table; no business logic
internal/<domain>/       # business logic, one package per concept; never imports grpc or codes
internal/store/          # only if the service has Postgres: queries
internal/events/         # only if the service publishes or consumes events
migrations/              # only if the service has Postgres: NNNN_name.up/down.sql and embed.go
.env.example             # at the service root, next to go.mod
```

`store` and `events` are optional; create them when needed. The directory is `rpc`, not `grpc`, so handler code imports `google.golang.org/grpc` without an alias. Run goimports with `local-prefixes` set to the service's module path (the SDK's `.golangci.yml` shows the setting).

### Settings and .env.example

`main` owns the process: it creates the signal context, loads settings, calls `app.Run` and exits non-zero on error. `app.Run` never touches signals, and tests build `Settings` by hand. This is `cmd/hello/main.go`:

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

`Settings`:

```go
type Settings struct {
	config.Runtime            // embed groups whose field names do not collide
	config.GRPC
	Postgres config.Postgres  // name groups whose field names collide (OTel and Postgres both used to have Enabled)
	OTel     config.OTel
}
func (s Settings) Validate() error { return s.Postgres.Validate() } // Load calls it
```

- `DATABASE_URL` is required: an empty one fails at load (`Postgres.Validate`). `GRPC_PORT` has no default and is required. Capabilities every deploy runs get no enable flag; only real toggles such as `OTEL_ENABLED` have one.
- `.env.example` lists every key once as `KEY=value`. Optional keys are written `KEY=` with no value. Commented lines are ignored by the drift check. The test lives in `internal/config` and reads `../../.env.example` with `config.CheckEnvExample`.
- Reflection is always on. A hand-built service description (as in `examples/hello`) has no descriptor, so reflection cannot describe its methods.

### Explicit wiring, no DI container

`internal/app.Run(ctx, settings) error` builds each dependency in order and passes it to the next constructor. No DI container, no reflection, no service locator. Build order: logger (then `slog.SetDefault`), telemetry, health, pool, server, `lifecycle.Run`. Register each closer on a `lifecycle.Stack` right after opening the resource. Closers run in reverse order, on shutdown (`Closers: {stack.Close}`) and on an early failure.

```go
func Run(ctx context.Context, s config.Settings) error {
	logger := logx.New(s.Runtime, service)
	slog.SetDefault(logger)
	var stack lifecycle.Stack
	shutdown, err := otelx.Init(ctx, s.OTel, service)
	if err != nil { return err }
	stack.Push(shutdown)
	health := healthx.New(healthx.DefaultInterval, logger)
	// pool, err := pgx.Open(...): unreachable => warn and go on only when ENV=local,
	//   otherwise return errors.Join(err, stack.Close(ctx))
	// health.Register("postgres", pgx.Checker(pool)); stack.Push(closePool)
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.NewHandler(logger))
	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(), DrainDelay: s.DrainDelay(),
		OnShutdown: []func(){health.Drain}, Closers: []func(context.Context) error{stack.Close}, Logger: logger,
	}, grpcx.Serve(srv, s.GRPC), lifecycle.Func(health.Run))
}
```

`examples/hello/internal/app/app.go` is the full version.

- Migrations: golang-migrate files `NNNN_name.up.sql` / `.down.sql` (four digits for new services; an existing service keeps its numbering) in `<repo>/migrations/`, applied by the deploy migrate job (`docker-compose.services.yaml`: `<svc>-migrate`), **not at boot**. A root package `migrations` (`migrations/embed.go`, `//go:embed *.sql`, exported `Files embed.FS`) embeds them so tests and an optional `migrate up` subcommand can pass `migrations.Files` to `migrate.Up`. `app.Run` only opens the pool. An unreachable database is tolerated only with `ENV=local` (warning, no checker); elsewhere it fails boot. A service applies migrations at boot only if its own ADR says so.
- Health: one `healthx.Health`, interval `healthx.DefaultInterval`, a component (`lifecycle.Func(health.Run)`), `OnShutdown: health.Drain`. A checker is named after its dependency (`"postgres"`).

### Run(ctx) error and lifecycle

Everything long-running (gRPC server, health poller, consumers, relays) is a component with `Run(ctx) error`. `lifecycle.Run` starts them in an errgroup, returns when the context ends or one fails, then shuts down in a fixed order: run `OnShutdown` hooks (drain health), wait `SHUTDOWN_DRAIN_DELAY_SECONDS` so probes and load balancers see NOT_SERVING, cancel components within `SHUTDOWN_GRACE_SECONDS`, then run the closers. `lifecycle.StopAfter(primary, dependents...)` keeps dependents (outbox relayer, sweeper) running until `primary` (the gRPC server) has returned, so in-flight RPCs that enqueue work are drained first.

`GracefulStop` waits for open streams, so a client holding a health `Watch` delays the stop until the grace period ends and the server is force-stopped. That is also how drain is tested: watch health, cancel, see NOT_SERVING while the stream is still open, then close the stream (`TestShutdownDrainsHealthFirst`).

### Errors are mapped in transport

Domain and store code return plain Go errors. Only `internal/rpc` converts them to gRPC status, through a per-service error table and `grpcx.Status`. A handler does three things: authorize, call the domain, map the error. Authorization (`auth.RequirePrincipal` or `RequireScopes`) is called in each handler; no interceptor enforces it.

```go
func (h *Handler) Greet(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	msg, err := greeter.Greet(in.GetValue())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs) // h.errs = ErrorTable{greeter.ErrEmptyName: codes.InvalidArgument}
	}
	return wrapperspb.String(msg + " (caller " + p.ID + ")"), nil
}
```

Matched errors keep their code, and errors that already are gRPC status errors (for example from `auth.RequirePrincipal`) pass through unchanged. Unknown errors are logged and returned as `codes.Internal` with the message `internal error`, so driver or upstream text never reaches callers.

### Tests

- App tests use an external package (`app_test`) and build `Settings` directly. `app.Run` needs a concrete port: take one from `net.Listen("127.0.0.1:0")` and release it, or build the server yourself and use `grpcx.ServeListener` on that listener.
- Use `logx.NewTo(io.Discard, runtime, "test")` where a logger is injected, and `testkit.Postgres(t)` for database tests. The service's `make check` (from `go-service.mk`) sets `GO_SDK_REQUIRE_DOCKER=1`, so those tests fail instead of skipping when Docker is missing.

### Libraries

| Concern | Library | Reason |
|---|---|---|
| Config | caarlos0/env v11 | Reads the `env:`/`default:` tags the services already use. |
| gRPC interceptors | go-grpc-middleware v2 | Maintained slog logging and recovery building blocks. |
| Concurrency | x/sync/errgroup | Standard way to run components and stop all on first error. |
| Postgres | pgx v5 | Already used by the services; pool, tx and ping built in. |
| Migrations | golang-migrate v4 | Matches the existing `NNN_name.up.sql` files. |
| Integration tests | testcontainers-go | Real Postgres in tests. Fakes have hidden real bugs before. |
| Lint | golangci-lint v2 | One shared config, pinned as a Go tool in the SDK module. |

Telemetry uses the OpenTelemetry Go SDK with OTLP/gRPC exporters and `otelgrpc` (ADR-0004). Retry, circuit breaking and feature flags are service-level choices (for example failsafe-go, OpenFeature), not SDK packages.

### What goes into the SDK

Code enters the SDK only when all three hold: it is substantial, at least three services need it, and it is hard to get right in each service. A small helper (a few lines, used by one or two services) stays in the service. Packages that fail this test are removed, as `flags`, `resilience` and `RequireScopesAllowAnonymous` were. Connection packages (redis, rate limit, quota, mongo, blob, queue, tasks, webhooks) land only when this rule holds.

### Borrowed from other frameworks

- From kratos: the component lifecycle (start together, stop in order) and the idea of one error model mapped to transport codes.
- From gofr: health that reflects each dependency, not a constant SERVING.

We do not adopt a framework. A framework would bring its own config, logging and routing conventions, and we need only the platform layer. We take the two patterns above and leave the rest. No DI container and no reflection because wiring is short and a compiler can check it.

### Distribution

The SDK is distributed the way ADR-0001 distributes proto.

- `platform-core` owns `packages/go-sdk` and versions it with git tags `go-sdk/vX.Y.Z`.
- A service vendors the tagged tree into `third_party/go-sdk` with `platform-core/tools/sync-go-sdk.sh <tag> <service-dir>`. The script writes `third_party/go-sdk/VERSION` (tag and commit), adds `replace github.com/buidangphuc/platform-core/packages/go-sdk => ./third_party/go-sdk` to `go.mod`, copies `make/go-service.mk` and `.golangci.yml` (with `local-prefixes` set to the service's module path), runs `buf generate` when the service has a `buf.gen.yaml`, adds a `tool` directive for golangci-lint at the version the SDK pins (`go get -tool`, so `go tool golangci-lint` works), and runs `go mod tidy`.
- The script checks the tag first and swaps the vendored directory in only after it is fully exported. A failed run changes nothing.
- An SDK change is a new tag. The vendored copy is never edited in place.
- A service moves to a newer tag only in a change that uses something the tag adds. A change that cuts an SDK tag never re-syncs services.
- `templates/go-service/Dockerfile` copies `third_party/` before `go mod download` so the replace resolves in the image build.

### Out of scope

AI concerns (LLM clients, RAG, Langfuse) stay in the Python services. This ADR covers the Go platform layer only.

## Open question

Task store: River (Postgres-backed) or RabbitMQ (ADR-0002) for background tasks. A task package enters the SDK only when the admission rule holds; until then each service picks its own.

## Consequences

- Config, logging, tracing, auth, health, lifecycle and Postgres behave the same in every service. A panic no longer kills a process. Health shows database loss.
- `auth.HasScope` no longer treats `admin` as a wildcard. Any admin policy is explicit service code.
- Services stay independent. A single repo clone builds without the others.
- Cost: each service carries a copy of the SDK and moves to a new tag only when it needs it. The drift check that compares the copy with its tag comes in phase 7.
- Cost: migration is one service at a time. Until a service migrates it keeps its own copy of this code.
- Revisit if we go remote: a private Go module proxy could replace vendoring without changing the tag scheme.
