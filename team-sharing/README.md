# team-sharing

Go gRPC service that owns the **short shareable links** bounded context: it mints a short code for a `(target_type, target_id)` pair, stores UTM parameters and Open Graph preview metadata with it, and counts every resolve. It is a deployed service (compose: `team-sharing-svc`, gRPC `:50065`, no host port published) with its own Postgres database `sharing_db` (Rule 3, database per service). It calls no other service and produces/consumes no events.

Go 1.22 (`go.mod`; the Dockerfile builds with `golang:1.22`), module `github.com/buidangphuc/team-sharing`.

Layout: `cmd/server` (wiring) -> `internal/grpcserver` -> `internal/interceptor` (principal) -> `internal/handler` -> `internal/service` (validation, short-code minting, OG defaults) -> `internal/repository` (Postgres + in-memory). `internal/config` reads env.

## Contract

Served: `platform.sharing.v1.SharingService` (`proto/platform/sharing/v1/sharing.proto`) on `:50065`, plus the standard gRPC health service (serving) and gRPC reflection (always on). The only caller is `team-gateway` (`UPSTREAM_SHARING_ADDR`, default `team-sharing-svc:50065`), which exposes it over Connect and forwards the principal as `x-principal-{id,type,scopes}` metadata.

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

Migrations are in `migrations/` (`001_create_share_links`, `002_share_links_created_by`, each with `.up.sql`/`.down.sql`). The service does not run them; in compose the one-shot `team-sharing-migrate` service (golang-migrate) applies them against the shared `postgres` container before `team-sharing` starts. For standalone runs apply them yourself.

## Configuration

The code reads only two env vars (`internal/config/config.go`; empty counts as unset):

| Var | Default | Meaning |
|---|---|---|
| `GRPC_PORT` | `50065` | gRPC listen port (the Dockerfile also sets it) |
| `DATABASE_URL` | `postgres://sharing_svc:sharing_pass@localhost:5447/sharing_db?sslmode=disable` | Postgres DSN |

`.env.example` also lists `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_REFLECTION_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `DATABASE_ENABLED`, `DB_MAX_CONNS`, `OTEL_*`. None of these is read by the code (see Known gaps); the compose service sets `DATABASE_ENABLED`, `ENV`, `GRPC_PORT`, `DATABASE_URL`, of which only the last two matter. There is no `.env.example` drift gate in this repo.

## Run locally

Root compose (from the repo root, `docker-compose.services.yaml`):

```
docker compose -f docker-compose.services.yaml up --build team-sharing
```

This brings up `postgres`, `team-sharing-migrate` and `team-sharing`. No port is published; reach it through `team-gateway` (`:8080`) or `docker exec`. There is no `jobs` profile for this service.

Standalone: start a Postgres with `sharing_db`/`sharing_svc`, apply `migrations/*.up.sql`, copy `.env.example` to `.env`, export the vars (the code does not load `.env` itself), then `make run`. Note `.env.example` points at port `5447`, while compose uses the in-network `postgres:5432`.

## Build, test and lint

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (remote buf.build plugins, needs network) |
| `make build` | `go build ./...` |
| `make test` | `go test ./...` (unit tests for config, handler, interceptor, service; repository has none) |
| `make run` | `go run ./cmd/server` |
| `make tidy` | `go mod tidy` |

There is no lint target and no CI config in this repo. A reasonable local gate: `make proto && go vet ./... && make test`.

## Spec and verification

- There is no `FEATURES.yaml` in this repo (peers such as `team-order` have one). End-to-end coverage lives in `platform-e2e`: share-link scenarios are in `tests/e2e/features/services/new_services.feature` and `tests/e2e/features/journeys/buyer_full_funnel_journey.feature`, with steps in `tests/e2e/step_definitions/test_new_services.py`.
- Verify with `make -C platform-e2e features-check` and, for an OpenSpec change, `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC. No open change currently targets sharing.

## Gotchas

- `proto/` is vendored from `platform-core/packages/proto` (ADR-0001) and is currently byte-identical to it. Never edit it here: change platform-core first, then re-sync.
- `generated/` is gitignored (root `.gitignore` `**/generated/`) and must be regenerated with `make proto` after a fresh checkout or proto re-sync, or the build fails.
- In-memory fallback: `cmd/server/main.go` falls back to an in-memory repo if `pgxpool.New` returns an error. That call is lazy and rarely fails, so an unreachable or misconfigured DB usually shows up as `Internal` errors on first query, not a fallback. When the fallback does trigger, links are lost on restart.
- Open Graph data is not supplied by callers. The service synthesizes a Vietnamese placeholder title/description and a `https://cdn.batdongsan.com.vn/og/<type>/<id>.png` image URL (`internal/service/sharing.go`, `defaultOgMeta`).
- The `utm` map is stored verbatim and returned on resolve; the service does not append it to any URL.

## Known gaps

- Inert env vars: everything in `.env.example` except `GRPC_PORT` and `DATABASE_URL` is ignored. Reflection is always on, the log handler is always JSON at default level, shutdown uses `GracefulStop` with no grace timeout, and no OTEL is wired.
- No authorization on either RPC (no scope checks, no `RequireScopes`). `CreateShareLink` is open to anonymous callers, so anyone can mint unlimited links; there is no rate limit in this service.
- `target_type`/`target_id` are not validated or checked for existence, and `target_type` is not restricted to known values.
- No expiry, deletion, or per-creator listing; `created_by` is stored but never read back through the API. There is no lookup-by-target despite the `idx_share_links_target` index.
- The proto comment mentions "UTM tracking and OG metadata" but the create request has no OG fields (OG is synthesized, see Gotchas). `team-gateway/README.md` lists a `TrackShare` RPC that does not exist in the proto.
- `repository` has no tests; no `FEATURES.yaml`; no CI config in the repo.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `platform-core/docs/ADR/`: [0001 proto distribution](../platform-core/docs/ADR/0001-proto-distribution.md), [0003 auth model](../platform-core/docs/ADR/0003-auth-model.md), [0006 RS256/JWKS](../platform-core/docs/ADR/0006-rs256-jwks-auth.md), [0010 service zero trust](../platform-core/docs/ADR/0010-service-zero-trust.md)
