## Why

team-sharing hand-rolls what SDK v1.0.2 now provides (config, logging, tracing, health, lifecycle, gRPC server chain, principal parsing, Postgres pool, migrations helper), and it still carries the runtime fallback the owner decided to delete (2026-10-22, `plans/go-sdk-migration/PLAN.md`, Step 1 and "Per-service recipe").

- `team-sharing/cmd/server/main.go` falls back to `repository.NewInMemoryShareLinkRepo()` when `pgxpool.New` fails. `pgxpool.New` connects lazily, so the fallback never triggers on an unreachable database and the service silently runs against a database it cannot use (ADR-0016 finding). When it does trigger, short links live only for the process and vanish on restart, while the gateway keeps returning codes that no replica can resolve.
- Config reads two variables, one with a hard-coded default DSN containing credentials (`internal/config/config.go`); `.env.example` lists variables the code never reads (`DATABASE_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `GRPC_REFLECTION_ENABLED`, `LOG_JSON`, `OTEL_*`) and there is no drift gate.
- Health is always SERVING, a stopped database is never reported, and no panic recovery, request id, structured call log or tracing exists in the gRPC chain.
- The service has a private copy of the principal interceptor and no shared lint or make rules.

Source of the requirements: `plans/go-sdk-migration/PLAN.md` (Step 1, per-service recipe, gap table row `sharing`), `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `0018-go-sdk-tagged-module-distribution.md`, and the `go-platform-sdk` spec on main including the archived `go-sdk-v1-0-1` requirements.

initiative: go-sdk-migration

## What Changes

- **Module**: `team-sharing/go.mod` requires `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2` (no `replace`, no `third_party/`). Packages used: `config`, `logx`, `otelx`, `healthx`, `lifecycle`, `grpcx`, `auth`, `postgres`, `migrate` (tests only, to apply `migrations/` to a throwaway database), `testkit` (tests only).
- **Layout (ADR-0016)**: `cmd/server/main.go` (signals, `config.Load`, `app.Run(ctx, settings)`, exit code) -> `internal/app` (composition root, `const service = "team-sharing"`) -> `internal/config` (`Settings`, `Load`) -> `internal/rpc` (was `internal/handler` + `internal/grpcserver`) -> `internal/sharelink` (was `internal/service`: validation, short-code minting, OG defaults; never imports grpc) -> `internal/store` (was `internal/repository`: Postgres repository; the in-memory repository moves to a `_test.go` fake). `internal/interceptor` is deleted: `auth.UnaryServerInterceptor` (via `grpcx.NewServer`) parses `x-principal-{id,type,scopes}`. `migrations/embed.go` embeds the SQL for tests; the service never applies migrations at boot (the compose `team-sharing-migrate` job and the deployed migrate job own them, unchanged).
- **BREAKING (boot)**: the service always uses Postgres. Boot fails when `DATABASE_URL` is empty (config load) or the database is unreachable within 5s (`postgres.Open`) in every `ENV`, including `local`. The in-memory fallback in `cmd/server/main.go` is deleted. Nothing in non-test code references an in-memory repository or `postgres.OpenOrLocal`.
- **BREAKING (config)**: `ENV` is required (no default; compose already sets `ENV=local`). `GRPC_PORT` is required (the Dockerfile already sets it). The default DSN is removed: `DATABASE_URL` is required. `DATABASE_ENABLED` is deleted everywhere and is not read. `SHUTDOWN_GRACE_SECONDS` is replaced by `SHUTDOWN_GRACE` (Go duration, default `10s`) and `SHUTDOWN_DRAIN_DELAY` (default `0s`); `GRPC_REFLECTION_ENABLED` is kept as an SDK variable (default false): reflection is OFF unless it is true, and a strict ENV refuses true (today reflection is always on). `.env.example` is rewritten to exactly the variables of `config.Settings` and a drift test (`TestEnvExampleInSync`, `make check-env`) is added.
- **Additive (observable)**: the readiness health service reports NOT_SERVING within one probe interval after the database is lost (today always SERVING) while the `liveness` health service (SDK v1.0.2) stays SERVING so the pod is not restarted; gRPC reflection is off by default; the SDK maps Canceled and DeadlineExceeded to their own codes and a unique violation (23505) to `AlreadyExists`; panics are recovered to `Internal`; every call is logged with request id, principal id and trace id; OTel is initialised when `OTEL_ENABLED=true`.
- **Unchanged**: proto, the two RPCs, their authorization (none required; anonymous accepted, creator recorded when authenticated), error codes and messages, 7-char base62 code, 5 collision retries, `share_links` schema and migrations 001 and 002, port 50065, image name, gateway contract, e2e scenario `A created share link resolves back to its target with OG meta`.
- **Build files**: `Makefile` becomes `SERVICE`, `GRPC_PORT`, `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, `include ../platform-core/make/go-service.mk` (no copies). `Dockerfile` builds `./cmd/server` with the module download needing no `third_party/`. README is rewritten for the new layout and config table.
- **Deploy lines deleted or changed** (listed in tasks 6 and 7): `docker-compose.services.yaml` `team-sharing.environment` loses `DATABASE_ENABLED=true`; `team-sharing/.env.example` loses `DATABASE_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `GRPC_REFLECTION_ENABLED`; `platform-gitops` gets `ENV` overlays for staging and prod (same way as the other services) and `probes.livenessService: liveness`; merge and sync the ENV overlays BEFORE the image bump, rollback is reverting the image tag.

## Non-goals

- Any change to `sharing.proto`, the gateway, the frontend `ShareButton` or the loop-event tracking requirements.
- `pkg/mockdata`, `auditx`, outbox, Kafka: team-sharing has no mock adapter, no privileged action, no events.
- Adding authorization to `CreateShareLink` or `ResolveShareLink` (anonymous by design) or rate limiting.
- Changing the SDK, the tag, or other services.

## Impact

`team-sharing/**` (go.mod, go.sum, Makefile, Dockerfile, .env.example, .gitignore, README.md, cmd/server, internal/*, migrations/embed.go; new FEATURES.yaml in the e2e track), `docker-compose.services.yaml` (one line), `platform-gitops/envs/**` (ENV overlays), `platform-e2e` (re-run only).

## Non-functional requirements

- Latency (non-gated target, no test asserts it): `CreateShareLink` and `ResolveShareLink` p95 unchanged (one Postgres statement each, the resolve increment staying atomic); the SDK chain adds under 1 ms per call. Boot adds one database ping, bounded at 5s by `postgres.Open`.
- Availability: a missing or unreachable database now fails boot (container restarts, readiness never green) instead of serving process-local links; with the compose `depends_on: team-sharing-migrate: service_completed_successfully` the database is up before boot. After boot, losing the database flips health to NOT_SERVING within one `healthx.DefaultInterval` and RPCs return `Internal` (`internal error`, no cause) until it returns; recovery needs no restart (pgx pool reconnects).
- Limits: `DB_MAX_CONNS` default 10; graceful shutdown deadline `SHUTDOWN_GRACE` default 10s; short code 7 chars from a 62-char alphabet; at most 5 mint attempts per create.
- Compatibility: wire contract and stored data are unchanged; no new third-party dependency beyond what the SDK pulls in.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `share_links.short_code` | public identifier (bearer capability) | resolve a shared link | contract: part of the share feature the user invokes | until the row is deleted (no TTL today; unchanged) |
| `share_links.target_type`, `target_id` | internal, opaque ids | what the link points to | contract | same as the row |
| `share_links.utm` (JSONB) | internal marketing attribution, no direct identifiers | campaign attribution | legitimate interest | same as the row |
| `share_links.og_title`, `og_description`, `og_image_url` | public preview text | unfurl previews | contract | same as the row |
| `share_links.click_count` | internal counter | share analytics | legitimate interest | same as the row |
| `share_links.created_by` | personal data (user id of the creator, `''` = anonymous) | attribute links to creators, abuse tracing | legitimate interest | same as the row; erasure of a user's links is NOT implemented (existing gap, unchanged) |
| Call logs (new, from `grpcx`) | principal id, request id, trace id, gRPC method, code | operations and incident response | legitimate interest | platform log retention (not set by this service) |

This change adds one new processing: structured call logs that carry the principal id. They SHALL NOT carry request bodies, short codes, `target_id`, `og_*`, `utm` values or error causes (the `rpc` error table still maps internal errors to `internal error`). No other personal data is added, moved or exposed.

## Threat model

- Assets: the `share_links` table (integrity of short-code to target mapping, `created_by` ids), availability of resolve for shared links, DSN credentials.
- Abuse cases and mitigations:
  - Code guessing or enumeration of `ResolveShareLink`: 62^7 (about 3.5e12) space, crypto/rand minting; unchanged, no rate limit here (gateway concern, see non-goals).
  - Spoofed identity: the only trusted source of the principal is `x-principal-*` metadata set by the gateway behind the ADR-0010 network policy (`allowFrom: [team-gateway]`); the legacy `x-user-id` header stays ignored. An anonymous or absent principal never gets a `created_by`.
  - Silent data loss: the removed in-memory mode meant a DB outage produced links that disappeared on restart and were unresolvable on other replicas. After this change the service refuses to boot and reports NOT_SERVING instead.
  - Credential leak: the default DSN with credentials is removed from code; the DSN comes only from the environment (Vault in deployed envs); logs never print the DSN or `err` text from the driver at `Internal` boundaries.
  - Error-text leak: handler errors for non-domain failures return `internal error` without the cause.
- Authorization rule: `CreateShareLink` and `ResolveShareLink` require no scope; anonymous callers are accepted by design. `created_by` is the forwarded principal id for non-anonymous callers and `''` otherwise. There is no per-user resource that another user must be refused, so no permission scenario applies beyond the spoofing case above.
