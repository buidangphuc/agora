# team-promotion

Go gRPC service (`:50061`) for the **promotions** bounded context. It owns shop and platform
**vouchers** (with an idempotent reserve/commit/release redemption seam used by checkout),
**flash-sale campaigns** (sale price plus a stock meter), **seller subscription plans**
(`FREE`/`PRO`/`PREMIUM`, entitlements lookup) and **sponsored ad slots**. It does discount
arithmetic and quota bookkeeping only. It never moves money (AGENTS.md section 7); `Subscribe` and
`CreateAdCampaign` are mocks that charge nothing. Status: deployed in the root compose.

## Contract

Protos live in `proto/` (vendored from `platform-core/proto`, package `platform.promotion.v1`).
Callers pass identity as gRPC metadata `x-principal-id`, `x-principal-type` (`user`, `service`,
`anonymous`) and `x-principal-scopes` (comma-separated). `internal/interceptor/auth.go` only parses
these into a principal; each handler enforces its own rule. `RequirePrincipal` rejects a missing,
`anonymous` or ANONYMOUS-type principal. `RequireAdmin` needs scope `admin`; `RequireSeller` needs
`listing.write` or `admin`.

| Service / RPC | Authorization (as implemented) |
|---|---|
| `VoucherService/CreateVoucher` | Principal required. Scope `PLATFORM` (and unspecified) needs `admin`. Scope `SHOP` needs `listing.write` or `admin`; `seller_id` is forced to the caller's id. |
| `VoucherService/GetVoucher` | None (public read by code). |
| `VoucherService/ListVouchers` | None. Filters by `seller_id` from the request (empty = platform). |
| `VoucherService/ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` | **None** (see Known gaps). `reservation_id` is required; the caller is expected to be team-order. |
| `FlashSaleService/CreateCampaign` | Principal required plus `listing.write` or `admin`. Non-admins must own the listing: team-domain `GetListing` (as the caller) must return `seller_id` equal to the caller id. Foreign listing is `PermissionDenied`, unknown listing `InvalidArgument`, lookup failure or `UPSTREAM_DOMAIN_ADDR` unset is `Unavailable` (fails closed). Admins skip the lookup. |
| `FlashSaleService/GetActiveFlashSale`, `ListActiveCampaigns`, `GetFlashSaleStock` | None (public reads). |
| `SubscriptionService/ListPlans` | None. |
| `SubscriptionService/Subscribe` | Any authenticated principal; seller id is the caller's id. |
| `SubscriptionService/GetEntitlements` | Principal required. A `user` principal may read only its own id; a `service` principal may read any seller. |
| `SponsoredService/CreateAdCampaign` | Any authenticated principal; seller id is the caller's id. |
| `SponsoredService/ListSponsoredSlots` | None (active campaigns, best bid first). |

Also served: `grpc.health.v1.Health` and gRPC reflection (`GRPC_REFLECTION_ENABLED`, default on).

**Consumes:** team-domain `ListingService/GetListing` (vendored `platform/listing/v1`), called with the
caller's forwarded principal, to verify listing ownership in `CreateCampaign` (`UPSTREAM_DOMAIN_ADDR`).
It also reads the principal metadata and depends on Postgres, Kafka and Flipt (below). In the root compose, `team-order` and `team-gateway` set
`UPSTREAM_PROMOTION_ADDR=team-promotion-svc:50061`.

Redemption flow: `ValidateAndReserve` is idempotent on `reservation_id` (a retry returns the
original hold; a released hold returns `valid=false`). It validates the time window, `min_spend`,
quota (`used < quota`) and, for shop vouchers, that the voucher `seller_id` equals the request
`seller_id`, then stores the discount. `CommitReservation` marks the hold committed and increments
`vouchers.used` once. `ReleaseReservation` frees an uncommitted hold. Discount: percent =
`subtotal*value/100` capped by `max_discount` (when > 0); fixed = `value`; always clamped to
`[0, subtotal]`.

## Events

Producer only; nothing is consumed.

| Topic | Type (`EventEnvelope.type`) | Key | Emitted when |
|---|---|---|---|
| `promotion.events` (`PROMOTION_EVENTS_TOPIC`) | `platform.promotion.v1.VoucherChanged` | voucher id | `CreateVoucher` succeeds |
| `promotion.events` | `platform.promotion.v1.FlashSaleChanged` | campaign id | `CreateCampaign` succeeds |

Value is a proto-marshalled `platform.events.v1.EventEnvelope` (event id, type, principal,
`traceparent`, `request_id`, payload). Emission is best-effort: failures are logged and never fail
the RPC. With `KAFKA_ENABLED=false` (the code default) or an unreachable broker at boot, no events
are published. See ADR-0002.

## Data

Own database `promotion_db` (user `promotion_svc`). Migrations in `migrations/` (golang-migrate,
`.up.sql`/`.down.sql`):

| Migration | Tables |
|---|---|
| `0001_promotion` | `vouchers`, `voucher_reservations` (unique `reservation_id`), `flash_sale_campaigns` |
| `0002_subscriptions` | `subscription_plans` (seeded `plan_free`, `plan_pro`, `plan_premium`), `seller_subscriptions` (unique `seller_id`) |
| `0003_sponsored` | `ad_campaigns` (index on `(status, bid DESC)`) |

Applied by the `team-promotion-migrate` one-shot container in the root compose (the app waits for
it), or by `make migrate` (golang-migrate via docker; `DATABASE_URL` from `.env` or the Makefile
default). The service itself does not run migrations.

## Configuration

Read in `internal/config/config.go` from the environment (struct tags are the source of truth).
`make check-env` runs `TestDeclaredEnvKeysNoDrift`, which fails if `.env.example` drifts from the
declared keys. The Makefile auto-loads `.env` if present.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `local` | `prod`/`production` sets `IsProd` |
| `LOG_LEVEL` | `info` | only `debug` changes behaviour; anything else is info |
| `LOG_JSON` | `true` | JSON vs text slog |
| `GRPC_HOST` | `0.0.0.0` | |
| `GRPC_PORT` | `50061` | |
| `GRPC_REFLECTION_ENABLED` | `true` | |
| `SHUTDOWN_GRACE_SECONDS` | `10` | graceful stop deadline |
| `DATABASE_ENABLED` | `true` | `false` selects in-memory repositories |
| `DATABASE_URL` | `""` | required when `DATABASE_ENABLED=true` (startup fails otherwise) |
| `DB_MAX_CONNS` | `10` | pgx pool size |
| `OTEL_ENABLED` | `false` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | `.env.example` uses `localhost:4317` |
| `OTEL_SERVICE_NAME` | `team-promotion` | |
| `FEATURE_FLAGS_ENABLED` | `true` | `false` skips Flipt; flags return the caller default |
| `FLIPT_ADDR` | `localhost:9000` | Flipt gRPC endpoint |
| `FEATURE_FLAGS_EVAL_TIMEOUT_MS` | `500` | |
| `KAFKA_ENABLED` | `false` | |
| `KAFKA_BROKERS` | `localhost:9092` | comma-separated |
| `PROMOTION_EVENTS_TOPIC` | `promotion.events` | |
| `UPSTREAM_DOMAIN_ADDR` | `""` | team-domain gRPC address (`team-domain-svc:50051` in compose). Unset: non-admin `CreateCampaign` fails closed; `.env.example` uses `localhost:50051` |

The only flag is `flash-sale-enabled` (fail-open, default true). Switched off, it makes
`GetActiveFlashSale` and `ListActiveCampaigns` return nothing.

## Run locally

Root compose (from the repo root; needs the `platform-core` infra up for `postgres`, `redpanda`
and `flipt` on the external `platform-core_default` network):

```bash
docker compose -f docker-compose.services.yaml up --build team-promotion
```

This runs `team-promotion-migrate` first, then the server on `:50061` with `DATABASE_URL` pointing
at the shared `postgres:5432` and Kafka and Flipt enabled. There is no `--profile jobs` component.

Standalone (from `team-promotion/`):

```bash
cp .env.example .env     # DATABASE_URL points at localhost:5438
make proto               # generated/ is gitignored; needs buf
make migrate             # needs a reachable Postgres
make run                 # go run ./cmd/server
```

With `DATABASE_ENABLED=false` the server boots on in-memory repositories (data lost on restart).

Smoke test (grpcurl; reflection is on by default):

```bash
grpcurl -plaintext -d '{"code":"SUMMER2026"}' localhost:50061 platform.promotion.v1.VoucherService/GetVoucher
grpcurl -plaintext -H 'x-principal-id: seller1' -H 'x-principal-type: user' -H 'x-principal-scopes: listing.write' \
  -d '{"code":"SHOP10","scope":"VOUCHER_SCOPE_SHOP","discount_type":"DISCOUNT_TYPE_PERCENT","discount_value":10}' \
  localhost:50061 platform.promotion.v1.VoucherService/CreateVoucher
grpcurl -plaintext localhost:50061 grpc.health.v1.Health/Check
```

## Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` into `generated/` |
| `make check` | merge gate: `check-env`, `gofmt -l .` (fails on unformatted files), `go vet ./...`, `go test ./...` |
| `make check-env` | `.env.example` drift test |
| `make test` | `go test ./...` |

Postgres repository tests (`internal/repository/postgres_test.go`) skip unless
`PROMOTION_TEST_DATABASE_URL` or `DATABASE_URL` is set. This repo has no `.github/workflows`;
`make check` is the gate to run before merging.

## Spec and verification

- `FEATURES.yaml` declares two features: `promotion.voucher-redeemed-at-checkout` and
  `promotion.flash-sale-live-stock`, both `status: automated`, covered by
  `platform-e2e/tests/e2e/features/promo/vouchers.feature`.
- Verify coverage with `make -C platform-e2e features-check` and, per change,
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Behaviour changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored (`buf generate`, ADR-0001). A fresh checkout does not compile until
  `make proto` has run. The Dockerfile does not run `buf`, so image builds need `generated/` present.
- `proto/` is vendored from `platform-core`. Never edit it here; change `platform-core/proto` and
  re-vendor.
- In-memory fallback: `DATABASE_ENABLED=false` swaps in in-memory repositories; a missing Kafka or
  Flipt at boot only logs a warning and degrades (no events, flags fail open).
- Port drift: standalone `.env.example` and the Makefile use Postgres `localhost:5438`; AGENTS.md
  lists `postgres-promotion` at `5440`. In compose the service uses the shared `postgres:5432`.
- The Dockerfile sets `GRPC_PORT=50060` and `EXPOSE 50060`, while the code default and compose use
  `50061`. Compose overrides it, so a bare `docker run` listens on 50060.

## Known gaps

- **Redemption RPCs are unauthenticated.** `ValidateAndReserve`, `CommitReservation` and
  `ReleaseReservation` have no principal or scope check, and `buyer_id` / `seller_id` come from the
  request body. Anyone who can reach the port can commit or release a hold.
- **`ListVouchers` and `GetVoucher` are public**, including by arbitrary `seller_id`.
- **`CreateAdCampaign` and `Subscribe` accept any authenticated principal** (buyers included); there is
  no seller-role gate.
- **`flash_sale_campaigns.stock_sold` is never incremented.** No code path updates it, so
  `GetFlashSaleStock` always reports `remaining == stock_cap`.
- **Quota counts commits only.** Outstanding (`reserved`) holds are not counted against `quota`, so
  concurrent reservations can exceed it before commits land.
- **No event on redemption or release.** `VoucherChanged` fires only on create, not when `used` changes.
- The `flash-sale-enabled` kill-switch only gates reads; it does not block `CreateCampaign`.

## Links

- Rules: [`../AGENTS.md`](../AGENTS.md); root ASDLC: [`../README.md`](../README.md)
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 auth model,
  0004 observability, 0008 inventory reservation model (mirrored by the reservation pattern),
  0010 service zero-trust
