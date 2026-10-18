## Why

The 15 Go services re-implement the platform layer (config, logging, tracing, auth, health, lifecycle, Postgres) in three drifting generations, and nothing enforces a shared shape:

- Identical copies of the config binder and `.env.example` check exist in 7–9 repos (`team-order/internal/config/envcheck.go`, `team-domain/internal/config/envcheck.go`, …). The OTLP tracer is copied into 6 repos (`team-domain/internal/observability/tracer.go:22`).
- No service recovers from a handler panic: no repo installs a recovery interceptor (`team-order/internal/grpcserver/server.go:24`). One nil dereference kills the process.
- `OTEL_*` settings are dead in team-order, payment and promotion. They install the `otelgrpc` stats handler, but no service initialises a tracer provider (`team-order/internal/interceptor/tracing.go:19`).
- Health is always SERVING in most repos, for example `team-order/internal/bootstrap/resources.go:28`. Only domain, identity and engagement probe the database (`team-domain/internal/bootstrap/lifecycle.go:198`).
- `team-sharing/cmd/server/main.go:32` treats `pgxpool.New` as a connection check. It is lazy, so the in-memory fallback never triggers.
- `platform-core/packages/go-sdk` exists but no service imports it. Its `auth.HasScope` lets the `admin` scope pass every check (`platform-core/packages/go-sdk/pkg/auth/auth.go:27`), which contradicts team-order's rule that bare `admin` does not open orders.

This change is phase 0 and phase 1 of the Go platform SDK initiative. It records the convention and the distribution model, then ships the core SDK. Later changes add the messaging and connection packages and migrate the services.

initiative: go-platform-sdk

## What Changes

- **ADR-0016, Go service convention and SDK distribution.** It fixes four things:
  - The repo layout: `cmd/`, `internal/app`, `internal/config`, `internal/rpc`, `internal/<domain>`, `internal/store`, `internal/events`.
  - Explicit wiring with no DI container. Everything long-running is a `Run(ctx) error`.
  - The library choices: caarlos0/env, go-grpc-middleware v2, x/sync/errgroup, pgx v5, golang-migrate v4, failsafe-go, testcontainers-go, golangci-lint v2.
  - Distribution by vendoring a tagged SDK copy, mirroring ADR-0001.
  
  The River vs RabbitMQ task-store question is recorded as open and is decided in phase 4.
- **Tooling** in platform-core:
  - `tools/sync-go-sdk.sh` vendors `packages/go-sdk` at a tag into a service's `third_party/go-sdk` and adds the `replace` directive.
  - `make/go-service.mk` holds the shared targets.
  - A shared `.golangci.yml`.
  - `templates/go-service/Dockerfile`.
- **Core SDK packages** in `platform-core/packages/go-sdk/pkg/`:
  - `reqctx`: request-id and principal-id context accessors.
  - `config`: rebuilt on caarlos0/env. It keeps the services' existing `env:`/`default:` tags and names, adds the `CheckEnvExample` drift check, and provides the groups `Runtime`, `GRPC`, `Postgres`, `OTel`.
  - `logx`: slog setup that adds request-id, principal-id and trace-id from ctx.
  - `otelx`: tracer and meter providers over OTLP/gRPC, a no-op when disabled, and the W3C propagator always set.
  - `auth`: rebuilt. Principal from `x-principal-*`, unary and stream interceptors, `RequirePrincipal`, and `RequireScopes` with exact matching.
  - `healthx`: a checker registry polled in the background that flips the gRPC health status, plus drain.
  - `lifecycle`: runs `Run(ctx) error` components under errgroup with an ordered, deadline-bound shutdown.
  - `pgx`: pool open + ping + health checker + `WithTx`.
  - `migrate`: golang-migrate runner over a service's `migrations/` directory.
  - `testkit`: a Postgres container fixture.
  - `resilience`: rebuilt on failsafe-go.
  - `grpcx`: the server builder with the interceptor chain recovery → request-id/logging → auth, otelgrpc, health and reflection, serving that drains on shutdown, and an error-to-status table.
- **An example service** (`packages/go-sdk/examples/hello`) wires every core package and is the reference the migration copies.
- **BREAKING** (only for code importing the old `go-sdk`, which is nothing today):
  - `auth.HasScope` no longer treats `admin` as a wildcard.
  - `config.Load` is replaced by `config.Load[T]()`.
  - `resilience.CircuitBreaker` is replaced by failsafe-go policies.

## Non-goals

- Migrating any `team-*` service. That starts with the phase 2 pilot (team-sharing, team-domain).
- Messaging and connection packages: Kafka, outbox, consumer, flags, idempotency, upstream, Redis, Mongo, S3, queue, tasks, webhooks.
- AI concerns: LLM clients, RAG and Langfuse stay in the Python services.
- Proto or contract changes, and CI wiring for the services. The CI drift gate comes in phase 7.

## Impact

- Repos touched: `platform-core` only (`packages/go-sdk`, `tools/`, `make/`, `templates/go-service/`, `docs/ADR/`). No running service changes behaviour.
- Capability: `go-platform-sdk` (new).
- No `FEATURES.yaml` update: the change is internal platform code with no user-facing behaviour.
- New Go module dependencies, all inside go-sdk:
  - caarlos0/env v11
  - grpc-ecosystem/go-grpc-middleware v2
  - golang.org/x/sync
  - go.opentelemetry.io SDK, OTLP exporters and otelgrpc
  - jackc/pgx v5
  - golang-migrate v4
  - failsafe-go
  - testcontainers-go (test-only use in `testkit`)

## Non-functional requirements

- A handler panic is converted to `codes.Internal` and the process keeps serving.
- With `OTEL_ENABLED=false`, telemetry adds no exporter, goroutine or network cost.
- Shutdown completes within `SHUTDOWN_GRACE_SECONDS`: health switches to NOT_SERVING, then the server stops gracefully, then dependencies close in reverse order. Past the deadline the server is force-stopped.
- Health reflects dependency reachability within one probe interval (default 5s).
- The SDK passes `go vet`, `golangci-lint` with the shared config, and `go test -race ./...`.

## Data classification

internal. The SDK reads principal ids, types and scopes forwarded by the gateway and may log principal ids and request ids. It stores no personal data. Logs carry only the opaque principal id, never tokens or scopes' secrets.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Principal identity in metadata | Spoofing: a caller sets `x-principal-*` directly | Unchanged trust model (ADR-0010): services are reachable only from the gateway network. `RequireScopes` never grants implicit access via `admin`. |
| Authorization decisions | Elevation of privilege: wildcard admin scope | Exact scope match; any admin policy is explicit service code. |
| Error responses | Information disclosure: driver or upstream error text in status messages | Unknown errors are mapped to `codes.Internal` with a generic message and logged server-side. |
| Service availability | Denial of service: a panic crashes the process | The recovery interceptor converts the panic to `Internal`; health probes switch NOT_SERVING on dependency loss. |
| Logs | Information disclosure: tokens or metadata dumped into logs | The logging interceptor logs method, code, duration, request-id and principal-id only, never request payloads or raw metadata. |
| Vendored SDK copy | Tampering: local edits drift from the tag | The sync script writes `VERSION`; the drift check (phase 7) compares the copy with its tag. |
