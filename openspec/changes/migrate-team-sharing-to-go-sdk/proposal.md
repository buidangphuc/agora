## Why

team-sharing is the smallest Go service and the minimal-tier pilot for go-sdk v0.1.0 (ADR-0016). Its platform layer has known defects:

- `team-sharing/cmd/server/main.go:32` treats `pgxpool.New` as a connectivity check. The pool is lazy, so an unreachable database never triggers the in-memory fallback, and every request then fails with `Internal`.
- No panic recovery, no request id, no tracing. Health is always SERVING (`team-sharing/internal/grpcserver/server.go:28`).
- `.env.example` declares about ten variables that `internal/config` ignores (`LOG_LEVEL`, `OTEL_*`, `DB_MAX_CONNS`, `SHUTDOWN_GRACE_SECONDS`, …).
- A server start error is only logged, and the process keeps waiting for a signal.

initiative: go-platform-sdk

## What Changes

- **Trim enable flags for always-on capabilities (go-sdk v0.1.1).** No deploy sets `DATABASE_ENABLED` or `GRPC_REFLECTION_ENABLED`, so both are always on. The SDK drops both: `DATABASE_URL` becomes required and reflection is always registered. `OTEL_ENABLED` stays, because every Go service except the gateway runs with telemetry off.
- Vendor go-sdk at tag `go-sdk/v0.1.1` with `platform-core/tools/sync-go-sdk.sh`, which writes `third_party/go-sdk`, adds the `replace` line and pins the lint tool.
- Restructure to the ADR-0016 layout:
  - `cmd/server/main.go` handles signals, `config.Load` and `app.Run`.
  - `internal/app` is the composition root.
  - `internal/config` holds Settings that embed the SDK groups, plus the `.env.example` drift test.
  - `internal/rpc` holds the handlers and the error table (was `internal/handler`).
  - `internal/sharelink` holds the domain (was `internal/service`).
  - `internal/store` holds the Postgres and in-memory stores (was `internal/repository`).
- Delete `internal/grpcserver` and `internal/interceptor`; their jobs move to SDK `grpcx` and `auth`. Callers who are anonymous are still allowed to create links; a small local helper returns the caller id, or "" for anonymous.
- Database behaviour:
  - Outside `ENV=local`, an unreachable database fails boot.
  - In `ENV=local`, it falls back to the in-memory store with a warning, as today.
  - Health reports NOT_SERVING when the database stops answering.
- Migrations stay in `team-sharing/migrations/`, unchanged. The compose job `team-sharing-migrate` (and any deploy migrate job) keeps applying them; the service does not migrate at boot.
- **Convention fix found by this pilot.** ADR-0016 and `examples/hello` put migrations under `internal/store/migrations` and apply them at boot. Every deployed service instead keeps `migrations/` at the repo root and applies it with a migrate job (`docker-compose.services.yaml`). This change amends ADR-0016 to say that, and moves the example's migrations to the root. The example exposes them through a root package, `migrations/embed.go` with `//go:embed *.sql`, so `migrate.Up` stays usable from a `migrate up` subcommand and from tests.
- Tooling:
  - `Makefile` includes the synced `make/go-service.mk`.
  - `Dockerfile` follows `platform-core/templates/go-service/Dockerfile`.
  - `.env.example` matches Settings exactly.
- Add `team-sharing/FEATURES.yaml` for this change's scenarios.

No RPC, proto or response changes. Status texts for auth errors become the SDK's `authentication required` / `missing scope`, but no team-sharing RPC requires auth today, so no caller sees a difference.

## Non-goals

- Changing share-link behaviour, the proto, or the gateway.
- Kafka or other messaging, which team-sharing does not use.
- Other services. team-domain is migrated in the separate change `migrate-team-domain-to-go-sdk`.

## Impact

- Repos: `team-sharing`, plus the platform-core ADR-0016 text and `packages/go-sdk/examples/hello` (migration location only).
- Capability: `sharing-runtime` (new).
- Deploy:
  - Outside local, the service now refuses to start without a reachable database, where today it starts and fails every request.
  - The image entrypoint stays `/app`-style per the template.
  - `GRPC_PORT` stays 50065.

## Non-functional requirements

- Shutdown drains health before the server stops, and stops within `SHUTDOWN_GRACE_SECONDS`.
- The OTEL variables work; telemetry stays off by default.

## Data classification

internal. `created_by` is an opaque principal id, as today, and no new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Share-link store | Denial of service: a panic crashes the process | SDK recovery interceptor |
| Error responses | Information disclosure: DB error text leaks | `grpcx.Status`; unknown errors become `internal error` |
| Creator attribution | Spoofing: forged `x-principal-id` | Unchanged trust model (ADR-0010): only the gateway reaches the service |
