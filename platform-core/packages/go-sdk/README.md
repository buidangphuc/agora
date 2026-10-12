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
| `config` | `Load[T]()` from env tags (calls `Validate()` if defined), groups `Runtime`, `GRPC`, `Postgres`, `OTel`, and the `CheckEnvExample` drift check. |
| `logx` | slog setup that adds request id, principal id and trace id from the context. |
| `otelx` | OTLP/gRPC tracer and meter providers; a no-op when disabled; always sets the W3C propagator. |
| `auth` | Principal from `x-principal-*` metadata, unary and stream interceptors, `RequirePrincipal`, `RequireScopes` (exact match). |
| `healthx` | Checker registry polled in the background; flips the gRPC health status; `Drain()` on shutdown. |
| `lifecycle` | Runs `Run(ctx) error` components together with an ordered, deadline-bound shutdown (optional drain delay), and `Stack` for closers. |
| `pgx` | Pool open with ping, health checker, `WithTx`. |
| `migrate` | golang-migrate runner over a service's `migrations/` directory. |
| `testkit` | Postgres container fixture for tests. |
| `resilience` | Retry and circuit-breaker policies on failsafe-go. |
| `grpcx` | Server builder (request-id, recovery, logging, auth, otelgrpc, health, reflection), serving that drains, and the error-to-status table. |

`examples/hello` wires all of the above and is the reference service.

## How a service adopts it

1. Pick a release tag, for example `go-sdk/v0.1.0`.
2. From the repo root, sync the SDK into the service:

   ```sh
   platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.0 team-<svc>
   ```

   This writes `third_party/go-sdk` (with `VERSION`), adds the `replace` to `go.mod`, copies `make/go-service.mk` and `.golangci.yml`, pins the golangci-lint `tool` directive to the SDK's version, and runs `go mod tidy`. It fails without changes if the tag does not exist. If the tag has no `.golangci.yml`, it prints a warning and skips that file.
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
4. Replace `internal/rpc/hello.go` (a hand-built `grpc.ServiceDesc`, used only to avoid codegen) with your generated registration, and `internal/greeter` with your domain packages. Handlers stay thin: `auth.Require*`, domain call, `grpcx.Status` with your `ErrorTable`. Set `GRPC_REFLECTION_ENABLED=true` only once the service has generated descriptors.
5. Drop `internal/store` if the service has no database, otherwise put `NNNN_name.up.sql`/`.down.sql` files in `internal/store/migrations`. Add `internal/events` only when needed.
6. Set goimports `local-prefixes` to your module path (the synced `.golangci.yml` has the SDK's value).
7. Tests: see ADR-0016 ("Tests") for ports, `grpcx.ServeListener`, `logx.NewTo` and `GO_SDK_REQUIRE_DOCKER`.
