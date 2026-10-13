## Why

team-referral is a minimal-tier Go service and part of wave A of the go-sdk migration (ADR-0016). It has the same platform defects that team-sharing had before #14:

- `team-referral/cmd/server/main.go:32` treats the lazy `pgxpool.New` as a connectivity check, so the in-memory fallback never triggers on an unreachable database.
- No panic recovery, no request ids, no tracing.
- Health is always SERVING.
- Its config ignores most `.env.example` keys.

initiative: go-platform-sdk

## What Changes

- Vendor go-sdk at `go-sdk/v0.1.1` and move to the ADR-0016 layout, following team-sharing:
  - `internal/app` is the composition root;
  - `internal/config` is rebuilt on SDK `config`;
  - `internal/handler` becomes `internal/rpc`;
  - `internal/service` becomes `internal/referral`;
  - `internal/repository` becomes `internal/store`;
  - `internal/grpcserver` and `internal/interceptor` are deleted (their jobs move to SDK `grpcx` and `auth`).
- Fail-fast database:
  - outside `ENV=local`, an unreachable database fails boot;
  - in `ENV=local`, the in-memory store is kept, with a warning;
  - health reports NOT_SERVING when the database stops answering.
- No enable flags: `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` are gone, and stale ones are removed from the team-referral compose block.
- `migrations/` stays at the repo root, applied by the compose migrate job, and gets an `embed.go`.
- The `Makefile` includes `make/go-service.mk`, and the `Dockerfile` comes from the template (`GRPC_PORT=50062`).

No RPC, proto or schema changes.

## Non-goals

- Behaviour changes.
- Other services.

## Impact

- Repo: `team-referral`, plus its compose block.
- Capability: `referral-runtime` (new).
- Deploy:
  - outside local, team-referral refuses to start without a reachable database;
  - `DATABASE_URL` and `GRPC_PORT` are required.

## Non-functional requirements

Shutdown drains health first and stays within `SHUTDOWN_GRACE_SECONDS`.

## Data classification

Unchanged from today's team-referral. No new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| team-referral availability | Denial of service: a panic crashes the process | SDK recovery interceptor |
| Error responses | Information disclosure: DB error text leaks | `grpcx.Status` |
