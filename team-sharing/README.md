# team-sharing

Go gRPC service that owns the **short shareable links** bounded context: it mints a short code for a `(target_type, target_id)` pair, stores UTM parameters and Open Graph preview metadata with it, and counts every resolve. It is a deployed service (compose: `team-sharing-svc`, gRPC `:50065`, no host port published) with its own Postgres database `sharing_db` (Rule 3, database per service). It calls no other service and produces/consumes no events.

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module `github.com/buidangphuc/team-sharing`. The platform layer (config, logging, tracing, auth, health, lifecycle, Postgres, gRPC server chain) is [go-sdk](../platform-core/packages/go-sdk/README.md) v0.1.0, vendored in `third_party/go-sdk` (see `third_party/go-sdk/VERSION`) by `platform-core/tools/sync-go-sdk.sh`; never edit it by hand. Layout follows [ADR-0016](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md):

| Path | Role |
|---|---|
| `cmd/server/main.go` | Signal context, `config.Load`, `app.Run`, exit code. |
| `internal/app` | Composition root: logger, telemetry, health, store, gRPC server, lifecycle. |
| `internal/config` | `Settings` (SDK groups) and the `.env.example` drift test. |
| `internal/rpc` | gRPC handlers and the error table; resolves the caller (`""` for anonymous). |
| `internal/sharelink` | Domain: validation, short-code minting, OG defaults. |
| `internal/store` | Postgres and in-memory share-link stores. |
| `migrations/` | SQL migrations plus `embed.go` (package `migrations`, for tests). |

## Contract

Served: `platform.sharing.v1.SharingService` (`proto/platform/sharing/v1/sharing.proto`) on `:50065`, plus the standard gRPC health service and gRPC reflection (always on). Health is SERVING while the database answers a ping, NOT_SERVING when it does not (checked every 5s) and during shutdown drain. A panic in a handler is recovered and returned as `Internal`; every call gets a request id and a log line. The only caller is `team-gateway` (`UPSTREAM_SHARING_ADDR`, default `team-sharing-svc:50065`), which exposes it over Connect and forwards the principal as `x-principal-{id,type,scopes}` metadata.

| RPC | Authorization | Behaviour |
|---|---|---|
| `CreateShareLink(target_type, target_id, utm)` | None required. Anonymous callers are accepted; if the forwarded principal is authenticated, its id is stored as `created_by` (`''` when anonymous). | `InvalidArgument` if `target_type` or `target_id` is empty. Mints a 7-char base62 code (crypto/rand), retries up to 5 times on collision. Returns `short_code`. |
| `ResolveShareLink(short_code)` | None, anonymous by design (anyone with the code can unfurl it). | `InvalidArgument` on empty code, `NotFound` on unknown code. Atomically increments `click_count` and returns target, UTM, `og_meta` and the post-increment `click_count`. |

Consumes: nothing (no upstream RPCs). `target_type`/`target_id` are opaque strings, never validated against the owning service.

## Events

None. There is no Kafka or RabbitMQ code in this repo.

## Data

Postgres database `sharing_db`, user `sharing_svc`, one table `share_links`:

| Column | Notes |
|---|---|
| `short_code` | `TEXT` primary key |
| `target_type`, `target_id` | `TEXT NOT NULL`; index `idx_share_links_target` on both |
| `utm` | `JSONB NOT NULL DEFAULT '{}'` |
| `og_title`, `og_description`, `og_image_url` | `TEXT NOT NULL DEFAULT ''` |
| `click_count` | `BIGINT NOT NULL DEFAULT 0` |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` |
| `created_by` | `TEXT NOT NULL DEFAULT ''`, added by migration 002; creator principal id, `''` = anonymous |

Migrations are in `migrations/` (`001_create_share_links`, `002_share_links_created_by`, each with `.up.sql`/`.down.sql`). The service does not run them at boot; in compose the one-shot `team-sharing-migrate` service (golang-migrate) applies them against the shared `postgres` container before `team-sharing` starts. For standalone runs apply them yourself. The root `migrations` package embeds the files for the store test.

## Configuration

`internal/config` binds the SDK groups from env; `.env.example` lists every key and `TestEnvExampleInSync` fails when the two drift. `ENV` and `GRPC_PORT` are required (no default). The Dockerfile sets `GRPC_PORT=50065`; every deployment must set `ENV` (compose and gitops do).

| Var | Default | Meaning |
|---|---|---|
| `ENV` | required | `local` or `test` allow the in-memory fallback; anything else requires a reachable database. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | Logging. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Listen address. |
| `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY` | `10s`, `0s` | Shutdown drains health first, then stops within the grace period. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | Postgres. A missing `DATABASE_URL` fails at load. |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | Telemetry, off by default. |

Database behaviour: the pool is opened with a ping. Outside `ENV=local` an unreachable database fails boot (non-nil error, port never opened). With `ENV=local` the service logs a warning and serves from the in-memory store.

## Run locally

Root compose (from the repo root, `docker-compose.services.yaml`):

```
docker compose -f docker-compose.services.yaml up --build team-sharing
```

This brings up `postgres`, `team-sharing-migrate` and `team-sharing`. No port is published; reach it through `team-gateway` (`:8080`) or `docker exec`. There is no `jobs` profile for this service.

Standalone: start a Postgres with `sharing_db`/`sharing_svc`, apply `migrations/*.up.sql`, copy `.env.example` to `.env`, export the vars (the code does not load `.env` itself), then `make run`. Note `.env.example` points at port `5447`, while compose uses the in-network `postgres:5432`. Without a database, run with `ENV=local GRPC_PORT=50065 DATABASE_URL=postgres://x@127.0.0.1:1/x make run`; the unreachable URL triggers the in-memory fallback.

## Build, test and lint

Targets come from `make/go-service.mk` (synced; `make help` lists them).

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (remote buf.build plugins, needs network) |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`; `DOCKER_HOST` is taken from the docker context |
| `make lint` | golangci-lint with the synced `.golangci.yml` |
| `make check` | vet, lint, tests (Docker tests must run, not skip), `.env.example` drift check |
| `make docker` | build the image `team-sharing:dev` |

Run `make proto` first on a fresh checkout and again after a proto re-sync, because `generated/` is gitignored. Tests that start Postgres use testcontainers, so Docker must be running; `make check` fails instead of skipping when it is not.

## Spec and verification

- `FEATURES.yaml` lists the runtime scenarios of change `migrate-team-sharing-to-go-sdk` as `not-testable` entries that name their unit tests. End-to-end coverage lives in `platform-e2e`: share-link scenarios are in `tests/e2e/features/services/new_services.feature` and `tests/e2e/features/journeys/buyer_full_funnel_journey.feature`, with steps in `tests/e2e/step_definitions/test_new_services.py`.
- Verify with `make -C platform-e2e features-check` and, for an OpenSpec change, `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `proto/` is vendored from `platform-core/packages/proto` (ADR-0001) and is currently byte-identical to it. Never edit it here: change platform-core first, then re-sync.
- `generated/` is gitignored (root `.gitignore` `**/generated/`) and must be regenerated with `make proto` after a fresh checkout or proto re-sync, or the build fails.
- In-memory fallback: only with `ENV=local` and an unreachable database. Links are then lost on restart, and health stays SERVING because there is no database to check.
- Open Graph data is not supplied by callers. The service synthesizes a Vietnamese placeholder title/description and a `https://cdn.batdongsan.com.vn/og/<type>/<id>.png` image URL (`internal/sharelink/sharelink.go`, `defaultOgMeta`).
- The `utm` map is stored verbatim and returned on resolve; the service does not append it to any URL.

## Known gaps

- No authorization on either RPC (no scope checks, no `RequireScopes`). `CreateShareLink` is open to anonymous callers, so anyone can mint unlimited links; there is no rate limit in this service.
- `target_type`/`target_id` are not validated or checked for existence, and `target_type` is not restricted to known values.
- No expiry, deletion, or per-creator listing; `created_by` is stored but never read back through the API. There is no lookup-by-target despite the `idx_share_links_target` index.
- The proto comment mentions "UTM tracking and OG metadata" but the create request has no OG fields (OG is synthesized, see Gotchas). `team-gateway/README.md` lists a `TrackShare` RPC that does not exist in the proto.
- No CI config in the repo.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `platform-core/docs/ADR/`: [0001 proto distribution](../platform-core/docs/ADR/0001-proto-distribution.md), [0003 auth model](../platform-core/docs/ADR/0003-auth-model.md), [0006 RS256/JWKS](../platform-core/docs/ADR/0006-rs256-jwks-auth.md), [0010 service zero trust](../platform-core/docs/ADR/0010-service-zero-trust.md)
