# team-sharing

Go gRPC service that owns the **short shareable links** bounded context: it mints a short code for a `(target_type, target_id)` pair, stores UTM parameters and Open Graph preview metadata with it, and counts every resolve. It is a deployed service (compose: `team-sharing-svc`, gRPC `:50065`, no host port published) with its own Postgres database `sharing_db` (Rule 3, database per service). It calls no other service and produces/consumes no events.

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module `github.com/buidangphuc/team-sharing`, built on `platform-core/packages/go-sdk` v1.0.2 (ADR-0016). The service always uses Postgres; there is no memory mode.

Layout: `cmd/server` (loads `internal/config`, calls `internal/app`) -> `internal/app` (wiring: logx, otelx, healthx, postgres, grpcx, lifecycle) -> `internal/rpc` (gRPC handler, principal metadata, error mapping) -> `internal/sharelink` (validation, short-code minting, OG defaults, `Repository` port) -> `internal/store` (Postgres).

## Contract

Served: `platform.sharing.v1.SharingService` (`proto/platform/sharing/v1/sharing.proto`) on `:50065`, plus the standard gRPC health service (overall status for readiness, `liveness` for the livenessProbe) and gRPC reflection only when `GRPC_REFLECTION_ENABLED=true` in a local or test `ENV`. The only caller is `team-gateway` (`UPSTREAM_SHARING_ADDR`, default `team-sharing-svc:50065`), which exposes it over Connect and forwards the principal as `x-principal-{id,type,scopes}` metadata.

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

`internal/config.Settings` is composed of the go-sdk config groups (`Runtime`, `GRPC`, `Postgres`, `OTel`) and loaded with `sdkconfig.Load`. `.env.example` lists exactly these variables; the code does not load `.env` itself.

| Var | Default | Meaning |
|---|---|---|
| `ENV` | none | Required. Deployment environment (`local`, staging, prod); strict environments refuse unsafe options |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_JSON` | `true` | JSON log output |
| `GRPC_HOST` | `0.0.0.0` | gRPC bind host |
| `GRPC_PORT` | `50065` | Required. gRPC listen port (the Dockerfile also sets it) |
| `GRPC_REFLECTION_ENABLED` | `false` | gRPC reflection; only allowed in a local `ENV` |
| `SHUTDOWN_GRACE` | `10s` | Graceful shutdown timeout |
| `SHUTDOWN_DRAIN_DELAY` | `0s` | Delay between NOT_SERVING and stop |
| `DATABASE_URL` | none (example: `postgres://sharing_svc:sharing_pass@localhost:5447/sharing_db?sslmode=disable`) | Required. Postgres DSN |
| `DB_MAX_CONNS` | `10` | Postgres pool size |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry export |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP endpoint |
| `OTEL_SERVICE_NAME` | `team-sharing` | OTel service name |

## Health

The standard gRPC health service reports readiness: it is NOT_SERVING when the database check fails (and while draining) and SERVING again on recovery. A separate `liveness` health service does not depend on the database and stays SERVING; the gitops chart points the liveness probe at it (`probes.livenessService: liveness`), so a database outage affects readiness only.

## Run locally

Root compose (from the repo root, `docker-compose.services.yaml`):

```
docker compose -f docker-compose.services.yaml up --build team-sharing
```

This brings up `postgres`, `team-sharing-migrate` and `team-sharing`. No port is published; reach it through `team-gateway` (`:8080`) or `docker exec`. There is no `jobs` profile for this service.

Standalone: start a Postgres with `sharing_db`/`sharing_svc`, apply `migrations/*.up.sql`, copy `.env.example` to `.env`, export the vars, then `make run`. Run the full local gate with `make check`. Note `.env.example` points at port `5447`, while compose uses the in-network `postgres:5432`.

## Build, test and lint

Targets come from `../platform-core/make/go-service.mk` (lint config from the go-sdk `.golangci.yml`); `make check` is the local gate. `make tidy` runs `go mod tidy`. Unit tests cover config, app, rpc, sharelink and store.

## Spec and verification

- Feature catalogue: `team-sharing/FEATURES.yaml`. End-to-end coverage lives in `platform-e2e`: share-link scenarios are in `tests/e2e/features/services/new_services.feature` and `tests/e2e/features/journeys/buyer_full_funnel_journey.feature`, with steps in `tests/e2e/step_definitions/test_new_services.py`.
- Verify with `make -C platform-e2e features-check` and, for an OpenSpec change, `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC. No open change currently targets sharing.

## Gotchas

- `proto/` is vendored from `platform-core/packages/proto` (ADR-0001) and is currently byte-identical to it. Never edit it here: change platform-core first, then re-sync.
- `generated/` is gitignored (root `.gitignore` `**/generated/`) and must be regenerated with `make proto` after a fresh checkout or proto re-sync, or the build fails.
- Open Graph data is not supplied by callers. The service synthesizes a Vietnamese placeholder title/description and a `https://cdn.batdongsan.com.vn/og/<type>/<id>.png` image URL (`internal/sharelink/sharelink.go`, `defaultOgMeta`).
- The `utm` map is stored verbatim and returned on resolve; the service does not append it to any URL.

## Known gaps

- No authorization on either RPC (no scope checks). `CreateShareLink` is open to anonymous callers, so anyone can mint unlimited links; there is no rate limit in this service.
- `target_type`/`target_id` are not validated or checked for existence, and `target_type` is not restricted to known values.
- No expiry, deletion, or per-creator listing; `created_by` is stored but never read back through the API. There is no lookup-by-target despite the `idx_share_links_target` index.
- The proto comment mentions "UTM tracking and OG metadata" but the create request has no OG fields (OG is synthesized, see Gotchas).

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `platform-core/docs/ADR/`: [0001 proto distribution](../platform-core/docs/ADR/0001-proto-distribution.md), [0003 auth model](../platform-core/docs/ADR/0003-auth-model.md), [0006 RS256/JWKS](../platform-core/docs/ADR/0006-rs256-jwks-auth.md), [0010 service zero trust](../platform-core/docs/ADR/0010-service-zero-trust.md)
