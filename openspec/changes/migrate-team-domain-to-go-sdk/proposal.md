## Why

team-domain is the source of the best platform patterns (DB health probe, outbox relayer, fail-fast DB) and the full-tier pilot for go-sdk v0.1.0 (ADR-0016). It still carries its own copies of the SDK's parts:

- `internal/config` binder + envcheck
- `internal/observability/tracer.go`
- `internal/interceptor/{auth,tracing}.go`
- `internal/grpcserver`
- `internal/bootstrap/{lifecycle,resources,addon}.go`

These copies diverge from the other services. They also have no panic recovery (`team-domain/internal/grpcserver/server.go`) and no drain-before-stop. `bootstrap.Addon` is a registry that ADR-0016 rejects in favour of explicit wiring.

initiative: go-platform-sdk

## What Changes

- Vendor go-sdk at `go-sdk/v0.1.0` with `platform-core/tools/sync-go-sdk.sh`.
- Restructure to the ADR-0016 layout:

  | From | To |
  |---|---|
  | `internal/handler` | `internal/rpc` |
  | `internal/service` | `internal/listing` (listings, bundles, storefront, reservation sweeper) |
  | `internal/repository` | `internal/store` |
  | `internal/storage` | `internal/objects` (S3 presigned URLs) |
  | `internal/events` | stays (publisher, relayer) |
  | `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/observability` | deleted, replaced by `internal/app` + SDK `grpcx`, `auth`, `healthx`, `lifecycle`, `otelx`, `logx`, `pgx` |

- The outbox relayer and the reservation sweeper become `lifecycle` components (`Run(ctx) error`) instead of goroutines with stop channels.
- The Kafka publisher and the pool are closed through `lifecycle.Stack`.
- The `bootstrap.Addon` registry is removed. It has no implementations beyond the extension point, and ADR-0016 requires explicit wiring.
- These stay as they are:
  - Every env var name and default, enforced by the `.env.example` drift test.
  - `DATABASE_ENABLED=true` being required.
  - `migrations/` at the repo root, applied by the compose job `team-domain-migrate` (not at boot).
- The `Makefile` includes `make/go-service.mk`, keeping the repo-specific `dev`/`stop`/`migrate` targets. The `Dockerfile` follows the template.

No RPC, proto, event or schema changes.

## Non-goals

- Replacing the hand-written outbox/relayer/publisher with SDK messaging packages. That is phase 3.
- Changing listing, bundle, storefront or reservation behaviour.
- team-sharing, which has its own change.

## Impact

- Repo: `team-domain` only.
- Capability: `domain-runtime` (new).
- Auth status texts become `authentication required` / `missing scope`, with the same codes. Gateway and e2e tests assert codes, not texts. The agent must confirm this with grep and report any text assertions.
- The shutdown order changes to: health drains → `SHUTDOWN_DRAIN_DELAY_SECONDS` → server stops → relayer/sweeper stop → publisher, then pool close.

## Non-functional requirements

- No regression in the existing 77 unit/integration tests or in the automated platform-e2e listing features.
- Shutdown stays within `SHUTDOWN_GRACE_SECONDS`.

## Data classification

internal (listings, stock, storefront metadata). No new data is collected, and logging stays free of payloads.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Listing write path | Denial of service: a handler panic kills the service | SDK recovery interceptor |
| Outbox events | Repudiation/loss: events dropped on shutdown | The relayer is a lifecycle component; it stops after the server and before the pool closes |
| Errors | Information disclosure: DB error text leaks | `grpcx.Status`; unknown errors become `internal error` |
