# platform-core

The shared foundation of the agora polyrepo. It is **not a service** and is not
deployed. It owns four things that every `team-*` / `platform-*` repo consumes:

| Owns | Where |
|---|---|
| The gRPC / Protobuf **contract** (single source of truth) | `packages/proto/` |
| The shared local **infra** (Postgres, Redis, Kafka-compatible broker, ...) | `infra/` |
| **Architecture docs and ADRs** (the rules) | `docs/`, `docs/ADR/` |
| Service **seed templates**, a small Go **SDK**, and dev/demo **tools** | `templates/`, `packages/go-sdk/`, `tools/` |

Bounded context: the platform contract and local runtime. It holds no business logic
and no database of its own.

## Contract

There are no RPCs served here. This repo defines them: 18 `.proto` files under
`packages/proto/platform/<domain>/v1/`.

| Proto package | Services defined |
|---|---|
| `common` | shared messages (incl. `Principal`), no service |
| `events` | `EventEnvelope` only, no service (see Events) |
| `identity` | `AuthService`, `AddressService`, `SessionService`, `PublicProfileService` |
| `listing` | `ListingService` |
| `search` | `SearchService` |
| `engagement` | `EngagementService` |
| `order` | `CartService`, `OrderService` |
| `payment` | `PaymentService` |
| `promotion` | `VoucherService`, `FlashSaleService`, `SubscriptionService`, `SponsoredService` |
| `chat` | `ChatService` |
| `notification` | `NotificationService` |
| `audit` | `AuditService` |
| `referral` | `ReferralService` |
| `sharing` | `SharingService` |
| `verification` | `VerificationService` |
| `ai` | `AIService` |
| `analytics` | `AnalyticsQueryService` |
| `recommendation` | `RecommendationService` |

Authorization is **not** declared in the proto. Each serving repo enforces scopes
(`RequireScopes`) on the `Principal` forwarded as `x-principal-*` gRPC metadata
(ADR-0003, ADR-0006). Consumers of this repo are all other repos, via the vendored
`proto/` copy.

Lint and breaking rules (`packages/proto/buf.yaml`): `STANDARD` lint with
`ENUM_ZERO_VALUE_SUFFIX` excepted; `FILE` breaking. Evolution is additive only: never
remove or renumber a field (`docs/CONTRACT_VERSIONING_GUIDE.md`, written in Vietnamese).

## Events

This repo defines only the wire envelope, `platform.events.v1.EventEnvelope`
(`packages/proto/platform/events/v1/events.proto`): `event_id`, `type` (fully qualified
proto name of the payload), `occurred_at`, `principal`, `traceparent`, `request_id`,
`payload` (bytes). Topic and key conventions are in ADR-0002 and ADR-0005 (for example
`listing.events`). It produces and consumes no events itself.

## Data

None owned. The local shared Postgres (`infra/`) is provisioned for the services:
one instance (`postgres:16-alpine`, host port 5432, superuser `postgres`/`postgres`).
`infra/postgres-init/10-create-service-databases.sh` runs once on first init of the
`pg_data` volume and creates, idempotently, one database and role per service:
`<svc>_db`, owner `<svc>_svc`, password `<svc>_pass`, for `identity listing search
engagement order payment chat notification promotion audit referral sharing
verification`. To add a service, append it to that script's `SERVICES` list, then
`make clean && make dev` (init scripts do not re-run on an existing volume).
Table migrations live in each service repo, not here.

## Configuration

This repo reads no application env vars. The Makefile takes none. Local infra
defaults are fixed in `infra/docker-compose.yaml`:

| Service | Image | Host port(s) |
|---|---|---|
| postgres | `postgres:16-alpine` | 5432 |
| redis | `redis:7-alpine` | 6379 |
| qdrant | `qdrant/qdrant:v1.19.0` | 6333 (http), 6334 (grpc) |
| flipt (feature flags) | `flipt/flipt:v1.54.0` | 8083 (HTTP + UI) |
| redpanda (Kafka API) | `redpandadata/redpanda:v24.1.7` | 19092 (host), 9644 (admin); in-network `redpanda:9092` |
| kafka-ui | `provectuslabs/kafka-ui:v0.7.2` | 8088 |
| rabbitmq | `rabbitmq:3-management` | 5672, 15672 |
| opensearch | `opensearchproject/opensearch:2.15.0` | 9200 |
| minio | `minio/minio:latest` | 9000 (S3), 9001 (console); `minio-init` creates public-read bucket `listing-images`; root `minioadmin`/`minioadmin` |
| jaeger | `jaegertracing/all-in-one:1.57` | 16686 |
| prometheus | `prom/prometheus:v2.52.0` | 9090 |
| grafana | `grafana/grafana:11.0.0` | 3001 |
| otel-collector | `otel/opentelemetry-collector-contrib:0.104.0` | 4317 (OTLP gRPC), 4318 (OTLP HTTP), 8889 (metrics) |

The compose comment ties the Qdrant pin to the `qdrant-client` 1.19 used by consumers;
bump them together. There is no `.env.example` and no drift gate here.

## Run locally

Whole stack from the monorepo root (project name `platform-core`; includes
`platform-core/infra/docker-compose.yaml`):

```bash
docker compose up -d --build                        # infra + every service
docker compose --profile jobs run --rm platform-recsys
```

Infra only, standalone from this directory:

```bash
make dev        # docker compose -f infra/docker-compose.yaml up -d
make stop       # down, keep volumes
make clean      # down -v: DESTROYS all local data
make help       # list targets
```

Both paths use the same compose project network, `platform-core_default`.

## Build, test and lint

Requires `buf` (`brew install bufbuild/buf/buf`) and `docker`; `make tools` checks both.

| Command | What it does |
|---|---|
| `make lint-proto` | `buf lint` in `packages/proto` |
| `make breaking` | `buf breaking` against `HEAD` of the **monorepo git root**, `subdir=platform-core/packages/proto`. Skipped if there is no git HEAD |
| `make check` | `lint-proto` + `breaking`. This is the merge gate |
| `make proto` | lint, breaking, then `buf generate` into `./gen` (gitignored) |
| `make hooks-install` | sets `core.hooksPath=.githooks`; the `pre-push` hook runs `make check` |

`buf generate` uses **remote BSR plugins** (`buf.build/...` in `buf.gen.yaml`), so it
needs network. Output goes only to `./gen/{go,python,jvm,ts}`, a verification artifact.
`packages/go-sdk` (`pkg/auth`, `pkg/config`, `pkg/resilience`) has its own `go.mod`
(Go 1.22) and unit tests; run `go test ./...` there. `tools/ci/standard-ci.yml` is a
reusable GitHub workflow template (contract, build-and-test, security jobs) that is not
wired to any remote.

## Spec and verification

There is **no `FEATURES.yaml`** in this repo: it has no user-facing capabilities.
Contract changes ride along with the OpenSpec change of the feature that needs them
(`openspec/changes/<id>` at the monorepo root, per the root README's ASDLC). Behaviour
is verified in `platform-e2e`: `make -C platform-e2e features-check` and
`make -C platform-e2e spec-check CHANGE=<id>`. See `SPEC_DRIVEN_WORKFLOW.md`.

Proto change recipe: edit `packages/proto` additively, `make check`, then re-vendor
`proto/` into each affected consumer and run `buf generate` there.

## Gotchas

- `gen/` is gitignored and absent until `make proto`.
- **The git root is the monorepo, not this directory.** Run `make check` anywhere inside
  the checkout; `make breaking` resolves the root and prefix itself. A standalone copy of
  this directory with no git history skips the breaking check.
- Consumers vendor `proto/` as a plain directory copy (for example `team-order/proto`)
  and gitignore their generated code. Never edit a vendored copy; edit here.
- `minio/minio:latest` and `minio/mc:latest` are unpinned.
- Postgres, MinIO and Grafana use default dev credentials; local only.
- Docs mix languages: `CONTRACT_VERSIONING_GUIDE.md` is Vietnamese.

## Known gaps

- **`tools/sync-proto.sh` is stale vs ADR-0001.** It `cp -R`s the proto into
  `team-domain`, `team-search`, `team-order`, `team-gateway`, `team-frontend` (a short,
  outdated list) and lints with `bufbuild/buf:latest`. ADR-0001 says platform-core never
  writes into sibling repos. Do not rely on it.
- **No `proto/vX.Y.Z` tags exist.** ADR-0001 specifies tag pinning via submodule or
  subtree; in practice consumers hold plain copies, so there is no enforced version pin.
- **`make dev-obs` is the same as `make dev`.** It passes `--profile observability`, but
  no service in `infra/docker-compose.yaml` declares a profile; Jaeger, Prometheus,
  Grafana and otel-collector always start. The `make dev` echo ("observability profile
  off by default") is inaccurate.
- `make dev` echoes only some ports; flipt, minio, kafka-ui, jaeger, prometheus, grafana
  and otel-collector are not listed (see the table above).
- `docs/ARCHITECTURE.md` still carries a "NOT here (Phase 0)" section.
- `tools/run-coverage-matrix.sh` hard-codes its service list. `tools/` also holds demo and
  seed scripts (`demo-golden-path.sh`, `seed*.sh`, `tools/seed/`,
  `run_live_user_journeys.py`, `tag_taxonomy_pipeline.py`, `build-all-images.sh`) that
  need the full stack and the gateway on `:8080`.

## Templates

`templates/_service-{go,python,jvm,ts}` are per-language service seeds, each with its own
README, Dockerfile and `proto-vendor/`. `_service-go` ships a gRPC server with auth and
tracing interceptors, health, reflection, and an in-memory `ListingService` seed; see
`templates/_service-go/README.md`.

## Docs and ADRs

Rules live in the monorepo root `AGENTS.md`. Start with `docs/ARCHITECTURE.md` (3 rules)
and `docs/AGENT_GUIDE.md`. Other docs: `ROADMAP`, `DATA_ARCHITECTURE`,
`INFRASTRUCTURE_ARCHITECTURE`, `SECURITY_AND_RELIABILITY`, `MASTER_CAPABILITY_MATRIX`,
`CONTRACT_VERSIONING_GUIDE`, `TAG_CLASSIFIER_AND_FILTER_ENRICHMENT`, `UI_SYSTEM_DESIGN`.

| ADR | Title | Status |
|---|---|---|
| 0001 | Proto distribution (pin + local-generate) | Accepted |
| 0002 | Async broker (Kafka + RabbitMQ) | Accepted |
| 0003 | Auth model (Principal + scopes) | Accepted |
| 0004 | Observability (OpenTelemetry) | Accepted |
| 0005 | Search read-model (OpenSearch) | Accepted |
| 0006 | RS256 + JWKS issuer/verifier split | Accepted |
| 0007 | Durable purchase saga + compensation | Accepted |
| 0008 | Inventory reservation model | Accepted |
| 0009 | Payment to Order event integration | Accepted |
| 0010 | Service-to-service zero-trust (interim) | Accepted (interim) |
| 0011 | Model serving (platform-modelserve) | Accepted |
| 0012 | Recommendation placement engine | Accepted |
| 0013 | Seller demand forecasting (platform-forecast) | Proposed |
| 0014 | Model registry and promotion gate | Accepted |
