# team-domain

Go gRPC service that owns the marketplace **listing write-model**: listings and variants, categories, seller storefronts (shop display name), bundles, and stock reservations. It is the bounded context behind `platform.listing.v1.ListingService` (CQRS write side, ADR-0005). Every listing write also records a `ListingChanged` event in a transactional outbox (ADR-0002) that a background relayer publishes to Kafka for read models such as `team-search`.

Status: deployed service (root `docker-compose.services.yaml`, `team-domain-svc:50051`). Only this service connects to its Postgres `listing_db`.

## Contract

Served: `platform.listing.v1.ListingService` (`proto/platform/listing/v1/listing.proto`), port `50051` (`GRPC_PORT`). The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes` (ADR-0003); this service does no JWT verification. Handlers enforce scopes with `interceptor.RequireScopes` / `RequireServiceScope` (`internal/interceptor/auth.go`). No principal gives `Unauthenticated`; a missing scope gives `PermissionDenied`.

| RPC | Authorization | Notes |
|---|---|---|
| `GetListing`, `ListListings`, `ListCategories`, `GetCategory` | `listing.read` | `NotFound` for unknown id |
| `ListMyListings` | `listing.write` | caller's own listings |
| `CreateListing` | `listing.write` | seller id forced from the principal |
| `UpdateListing`, `DeleteListing` | `listing.write` + owner, or `admin` scope | `PermissionDenied` "not the listing owner"; delete is a hard `DELETE` |
| `GetImageUploadUrl` | `listing.write` | `Unavailable` if no object store |
| `UpsertStorefront` | `listing.write` | row keyed by the principal; `AlreadyExists` on slug clash |
| `GetStorefront`, `BatchGetStorefronts` | `listing.read` | batch max 100 ids, else `InvalidArgument` |
| `CreateBundle` | `listing.write` | owner-scoped |
| `GetBundle`, `ListBundlesBySeller` | `listing.read` | |
| `ReserveStock`, `ReleaseStock`, `CommitReservation` | **SERVICE principal** (type `service`) holding `listing.write` | internal RPCs; team-order calls them as `service-team-order`. A user, even a seller with the scope, gets `PermissionDenied` |

Reservation lifecycle (spec `inventory-reservations`, ADR-0008): `active → committed | released`, `committed → released`. Only `active` reservations expire; stock is restored once, when a reservation becomes `released`, by the quantity stored on it.

- `ReserveStock` requires `reservation_id` (`InvalidArgument` when empty; there is no ledger-less decrement). The id of an `active`/`committed` reservation is an idempotent success; the id of a `released` one is `FailedPrecondition` (reserve under a new id). Out of stock returns OK with `success=false, message="insufficient stock"` (not an error code). Unknown listing or variant gives `NotFound`.
- `CommitReservation` makes an `active` reservation `committed` (never swept). Repeat: OK. Released: `FailedPrecondition`. Unknown: `NotFound`. Empty id: `InvalidArgument`.
- `ReleaseStock` is keyed by `reservation_id` only (`InvalidArgument` when empty); `listing_id`, `variant_id` and `quantity` are ignored. It restores the stored quantity of an `active` or `committed` reservation exactly once; a repeated, already swept or unknown id is a successful no-op (WARN log).

Also serves gRPC health, and reflection when `GRPC_REFLECTION_ENABLED=true`.

Callers (root compose): `team-gateway` (`UPSTREAM_LISTING_ADDR`), `team-order` and `team-notification` (`UPSTREAM_DOMAIN_ADDR`). Consumes nothing upstream.

## Events

| Direction | Topic | Type | Key |
|---|---|---|---|
| Produces | `listing.events` (`KAFKA_LISTING_TOPIC`) | `platform.listing.v1.ListingChanged` (`CHANGE_TYPE_CREATED/UPDATED/DELETED`) in a `platform.events.v1.EventEnvelope` | listing id |
| Produces | `listing.events` | `platform.listing.v1.ListingStockChanged` in a `platform.events.v1.EventEnvelope` | listing id |
| Consumes | none | | |

`ListingChanged` is emitted on Create/Update/Delete. `ListingStockChanged` is written to the outbox inside every reserve, release and TTL-sweep transaction that changes stock: one per affected listing, with the listing's stock after the change and only the variants that changed. Idempotent no-ops (repeat reserve, repeated or unknown release, commit) emit nothing. `ListingBaseInfoChanged`, `ListingPricingChanged` and `ListingStatusChanged` exist in the proto but are reserved.

Outbox: the listing write and its outbox row commit in one DB transaction. The relayer (`internal/events/relayer.go`) claims pending rows with `FOR UPDATE SKIP LOCKED`, produces the stored envelope, and marks rows `published`. Delivery is at-least-once; consumers dedupe on `EventEnvelope.event_id` (the outbox row id). Failures retry with exponential backoff (1s doubling, capped at 5m); after `OUTBOX_MAX_ATTEMPTS` the row is parked as `failed`. The relayer only runs when `OUTBOX_ENABLED` and `KAFKA_ENABLED` are both true; with Kafka off, rows stay `pending`.

## Data

Postgres `listing_db` (role `listing_svc`). Migrations in `migrations/` (golang-migrate, `NNNN_name.up/down.sql`):

| Migration | Adds |
|---|---|
| 0001 `init` | `listings` |
| 0002 `seller` | `listings.seller_id` (owner) |
| 0003 `images` | `listings.image_keys` |
| 0004 `categories` | `categories` |
| 0005 `variants` | `listing_variants` |
| 0006 `outbox_events` | `outbox_events` (status `pending/published/failed`, attempts, `available_at`, `locked_until`) |
| 0007 `reservations` | `reservations` (idempotency key, `expires_at` TTL, `active/released`) |
| 0008 `storefronts` | `storefronts` |
| 0009 `bundles` | `bundles` |
| 0010 `reservation_lifecycle_constraints` | `NOT VALID` checks: `reservations.status IN ('active','committed','released')`, `listings.stock >= 0`, `listing_variants.stock >= 0`. Validate later with `psql "$DATABASE_URL" -f scripts/validate_0010_constraints.sql` (refuses unless every violation count is 0) |

Applied by `make migrate` locally, or by the one-shot `team-domain-migrate` container in the root compose (the service waits for it to complete). A reservation sweeper (`internal/service/sweeper.go`, tick `RESERVATION_SWEEP_INTERVAL`, TTL `RESERVATION_TTL`; defaults 1m / 15m) runs inside this service when Postgres is enabled and restores stock for expired `active` reservations only (committed ones are never swept).

## Configuration

Read in `internal/config/config.go` (struct tags are the source of truth). `make check-env` runs `TestEnvExampleInSync`, which fails if `.env.example` drifts from the declared fields in either direction.

| Var | Default |
|---|---|
| `ENV` | `local` |
| `LOG_LEVEL` | `info` |
| `LOG_JSON` | `true` |
| `GRPC_HOST` | `0.0.0.0` |
| `GRPC_PORT` | `50051` |
| `GRPC_REFLECTION_ENABLED` | `true` |
| `SHUTDOWN_GRACE_SECONDS` | `10` |
| `DATABASE_ENABLED` | `true` |
| `DATABASE_URL` | empty; required when `DATABASE_ENABLED=true` |
| `DB_MAX_CONNS` | `10` |
| `STORAGE_ENDPOINT` | `localhost:9000` |
| `STORAGE_BUCKET` | `listing-images` |
| `STORAGE_ACCESS_KEY` / `STORAGE_SECRET_KEY` | `minioadmin` / `minioadmin` |
| `STORAGE_REGION` | `us-east-1` |
| `STORAGE_USE_SSL` | `false` |
| `STORAGE_PUBLIC_BASE_URL` | `http://localhost:9000/listing-images` |
| `KAFKA_ENABLED` | `false` (no-op producer) |
| `KAFKA_BROKERS` | `localhost:9092`, comma-separated |
| `KAFKA_LISTING_TOPIC` | `listing.events` |
| `OUTBOX_ENABLED` | `true` |
| `OUTBOX_POLL_INTERVAL` | `1s` (Go duration) |
| `OUTBOX_BATCH_SIZE` | `100` |
| `OUTBOX_CLAIM_LOCK_SECONDS` | `60` |
| `OUTBOX_MAX_ATTEMPTS` | `10` |
| `RESERVATION_TTL` | `15m` (Go duration): how long an uncommitted (`active`) reservation holds stock before the sweeper restores it |
| `RESERVATION_SWEEP_INTERVAL` | `1m` (Go duration): sweeper period |
| `OTEL_ENABLED` | `false` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty |
| `OTEL_SERVICE_NAME` | `team-domain` |

`RESERVATION_TTL` and `RESERVATION_SWEEP_INTERVAL` never fail boot: a missing, unparsable or non-positive value falls
back to the default with a WARN naming the variable. The sweeper logs `reservation sweeper started` with the effective
`reservation_ttl` and `sweep_interval` (e.g. `15m0s`, `1m0s`).

## Run locally

Full stack (from the repo root; `docker-compose.yaml` includes `platform-core/infra` for Postgres and Redpanda plus `docker-compose.services.yaml`):

```
docker compose up -d --build team-domain
```

This builds the image, runs `team-domain-migrate`, and starts `team-domain-svc` with `KAFKA_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`. The Dockerfile needs `./generated` to exist, so run `make proto` first. There is no `--profile jobs` component.

Standalone:

```
cp .env.example .env
make dev        # Postgres on localhost:5433 via docker-compose.local.yaml
make migrate    # golang-migrate in docker, against DATABASE_URL
make run        # go run ./cmd/server (Kafka off unless you set KAFKA_ENABLED=true)
```

If you enable Kafka standalone, point `KAFKA_BROKERS` at a reachable broker (platform-core's Redpanda exposes host port 19092). `make stop` tears down the local Postgres.

## Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs `buf`; AGENTS.md says to run it in Docker) |
| `make test` | `go test ./...` |
| `make check-env` | `.env.example` drift gate |
| `make check` | merge gate: `check-env`, `gofmt -l`, `go vet ./...`, `golangci-lint run` (skipped if not installed), `go test ./...` |

`make check` is the declared gate; run it before a PR. The repo has no `.github` workflows.

## Spec and verification

- `FEATURES.yaml` (repo root): `listing.create`, `listing.edit`, `listing.delete`, `listing.shop-display-name` are `automated`; `listing.stock-reservation` and `listing.event-durability` are `not-testable` (no e2e).
- E2E coverage lives in `platform-e2e`: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>`.
- Behaviour changes go through OpenSpec (`openspec/changes/<id>` at the repo root), per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored and produced by `make proto`; never hand-edit or commit it. Build and tests fail until it exists.
- `proto/` is a vendored, pinned copy of platform-core's proto module (ADR-0001). Never edit it here; re-vendor at a new tag (see `proto/README.md`).
- With `DATABASE_ENABLED=false` there is no outbox and no sweeper (event-less service). With `KAFKA_ENABLED=false` writes succeed but events are never relayed.
- `ListingChanged` carries the full listing; there are no fine-grained change events.

## Known gaps

- The four fine-grained listing events (`BaseInfo`, `Pricing`, `Stock`, `Status` changed) are defined in the proto but never emitted.
- No service-to-service authentication beyond trusting `x-principal-*` metadata: a caller that reaches port 50051 directly can forge a service principal (ADR-0010 adds only an interim NetworkPolicy).
- `listing.stock-reservation` and `listing.event-durability` have no e2e coverage.
- The `FEATURES.yaml` note says the reservation TTL is swept by team-order; the sweeper actually runs in this service.

## Links

- Root rules: `../AGENTS.md`
- ADRs (`../platform-core/docs/ADR/`): 0001 proto distribution, 0002 async broker / outbox, 0003 auth model, 0004 observability, 0005 search read model (CQRS), 0007 durable saga, 0008 inventory reservation, 0010 service zero-trust
