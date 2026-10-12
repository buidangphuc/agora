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

Every Go service has this shape:

```
cmd/<binary>/main.go     # under 20 lines: load config, call app.Run
internal/app/            # composition root: builds and wires everything
internal/config/         # settings struct embedding the SDK groups
internal/rpc/            # gRPC handlers (package rpc) and the error table; no business logic
internal/<domain>/       # business logic, one package per domain concept
internal/store/          # Postgres access and migrations
internal/events/         # event publishing and consuming
```

### Explicit wiring, no DI container

`internal/app.Run(ctx) error` builds each dependency in order and passes it to the next constructor. There is no DI container, no reflection and no service locator. Reading `app.go` top to bottom shows what runs.

### Run(ctx) error and lifecycle

Everything long-running (gRPC server, health poller, consumers, relays) is a component with `Run(ctx) error`. `lifecycle.Run` starts them in an errgroup, returns when the context ends or one fails, then shuts down in a fixed order: drain health, cancel components within `SHUTDOWN_GRACE_SECONDS`, close dependencies in reverse order.

### Errors are mapped in transport

Domain and store code return plain Go errors. Only `internal/rpc` converts them to gRPC status, through a per-service error table and `grpcx.Status`. The directory is `rpc`, not `grpc`, so handler code can import `google.golang.org/grpc` without an alias. Matched errors keep their code, and errors that already are gRPC status errors (for example from `auth.RequirePrincipal`) pass through unchanged. Unknown errors are logged and returned as `codes.Internal` with the message `internal error`, so driver or upstream text never reaches callers.

### Libraries

| Concern | Library | Reason |
|---|---|---|
| Config | caarlos0/env v11 | Reads the `env:`/`default:` tags the services already use. |
| gRPC interceptors | go-grpc-middleware v2 | Maintained slog logging and recovery building blocks. |
| Concurrency | x/sync/errgroup | Standard way to run components and stop all on first error. |
| Postgres | pgx v5 | Already used by the services; pool, tx and ping built in. |
| Migrations | golang-migrate v4 | Matches the existing `NNN_name.up.sql` files. |
| Resilience | failsafe-go | Composable retry and breaker policies; replaces the hand-written breaker. |
| Integration tests | testcontainers-go | Real Postgres in tests. Fakes have hidden real bugs before. |
| Lint | golangci-lint v2 | One shared config, pinned as a Go tool in the SDK module. |

Telemetry uses the OpenTelemetry Go SDK with OTLP/gRPC exporters and `otelgrpc` (ADR-0004).

### Borrowed from other frameworks

- From kratos: the component lifecycle (start together, stop in order) and the idea of one error model mapped to transport codes.
- From gofr: health that reflects each dependency, not a constant SERVING.

We do not adopt a framework. A framework would bring its own config, logging and routing conventions, and we need only the platform layer. We take the two patterns above and leave the rest. No DI container and no reflection because wiring is short and a compiler can check it.

### Distribution

The SDK is distributed the way ADR-0001 distributes proto.

- `platform-core` owns `packages/go-sdk` and versions it with git tags `go-sdk/vX.Y.Z`.
- A service vendors the tagged tree into `third_party/go-sdk` with `platform-core/tools/sync-go-sdk.sh <tag> <service-dir>`. The script writes `third_party/go-sdk/VERSION` (tag and commit), adds `replace github.com/buidangphuc/platform-core/packages/go-sdk => ./third_party/go-sdk` to `go.mod`, copies `make/go-service.mk` and `.golangci.yml`, adds a `tool` directive for golangci-lint at the version the SDK pins (`go get -tool`, so `go tool golangci-lint` works), and runs `go mod tidy`.
- The script checks the tag first and swaps the vendored directory in only after it is fully exported. A failed run changes nothing.
- An SDK change is a new tag. Services bump deliberately in a PR. The vendored copy is never edited in place.
- `templates/go-service/Dockerfile` copies `third_party/` before `go mod download` so the replace resolves in the image build.

### Out of scope

AI concerns (LLM clients, RAG, Langfuse) stay in the Python services. This ADR covers the Go platform layer only.

## Open question

Task store: River (Postgres-backed) or RabbitMQ (ADR-0002) for background tasks. It is decided in phase 4, when the messaging packages are built. Phases 0 and 1 do not depend on it.

## Consequences

- Config, logging, tracing, auth, health, lifecycle and Postgres behave the same in every service. A panic no longer kills a process. Health shows database loss.
- `auth.HasScope` no longer treats `admin` as a wildcard. Any admin policy is explicit service code.
- Services stay independent. A single repo clone builds without the others.
- Cost: each service carries a copy of the SDK and must re-sync on upgrade. The drift check that compares the copy with its tag comes in phase 7.
- Cost: migration is one service at a time. Until a service migrates it keeps its own copy of this code.
- Revisit if we go remote: a private Go module proxy could replace vendoring without changing the tag scheme.
