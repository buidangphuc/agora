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
| `config` | `Load[T]()` from env tags (calls `Validate()` if defined; `keepempty:"true"` keeps an explicitly empty value), groups `Runtime` (`IsLocal`, `IsStrict`), `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Flags`, `PositiveDuration`, and the `CheckEnvExample` drift check. |
| `logx` | slog setup that adds request id, principal id and trace id from the context. |
| `otelx` | OTLP/gRPC tracer and meter providers; a no-op when disabled; always sets the W3C propagator. |
| `auth` | Principal from `x-principal-*` metadata, interceptors, type constants, `RequirePrincipal`, `RequireScopes` (exact match), `RequireScopesAllowAnonymous`, `RequireService`, `RequireUser`. |
| `jwtx` | JWT claims and JWKS wire types shared by issuer and verifier. |
| `idempotency` | The `idempotency-key` metadata key and its validation rule. |
| `healthx` | Checker registry polled in the background; flips the gRPC health status; `Drain()` on shutdown. |
| `lifecycle` | Runs `Run(ctx) error` components with an ordered, deadline-bound shutdown, `Stack` for closers, `StopAfter`, `Every` (ticker worker) and `HTTP` (graceful HTTP server). |
| `pgx` | Pool open with ping, `OpenOrLocal` (in-memory fallback only when local), health checker, `WithTx`. |
| `migrate` | golang-migrate runner over a service's `migrations/` directory. |
| `pagination` | `Clamp` and opaque offset page tokens. |
| `grpcx` | Server builder (request-id, recovery, logging, auth, otelgrpc, health, reflection), serving that drains, and `Status` with an `ErrorTable` of `Rule{Code, Message}`. |
| `kafkax` | franz-go producer and consumer clients with trace and request-id headers; an empty group gives a group-less reader. |
| `outbox` | Transactional outbox table, idempotent `EnqueueTx`, and a relayer with retry and park. |
| `consume` | Per-record and batch consumers with DLQ and `Permanent`; `Dedupe` (`Seen`, `Processed`). |
| `upstream` | gRPC client dial with principal and request-id propagation, `AsService`, per-call service principal, `WithDialOptions`. |
| `resilience` | Retry (with `RetryIf`) and circuit-breaker policies on failsafe-go. |
| `flags` | OpenFeature client over Flipt that fails open; `NewOrDisabled`. |
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
