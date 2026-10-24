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
`anonymous`) and `x-principal-scopes` (comma-separated). The SDK auth interceptor (chained by `grpcx.NewServer`) only parses
these into a principal; each RPC handler in `internal/rpc` enforces its own rule. `RequirePrincipal` rejects a missing,
`anonymous` or ANONYMOUS-type principal. `RequireAdmin` needs scope `admin`; `RequireSeller` needs
`listing.write` or `admin`.

| Service / RPC | Authorization (as implemented) |
|---|---|
| `VoucherService/CreateVoucher` | Principal required. Scope `PLATFORM` (and unspecified) needs `admin`. Scope `SHOP` needs `listing.write` or `admin`; `seller_id` is forced to the caller's id. |
| `VoucherService/GetVoucher` | None (public read by code). |
| `VoucherService/ListVouchers` | None. Filters by `seller_id` from the request (empty = platform). |
| `VoucherService/ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` | `Commit`/`Release`: SERVICE principal holding `promotion.reserve` only (user/admin tokens are `PermissionDenied`). `ValidateAndReserve` is dual-mode: a SERVICE principal with `promotion.reserve` is trusted (request `buyer_id`/`seller_id` honoured); any other authenticated principal gets `buyer_id` forced to its own id and `reservation_id` must start `preview:<its id>:` (else `PermissionDenied`), so a preview hold can never be committed. |
| `FlashSaleService/CreateCampaign` | Principal required plus `listing.write` or `admin`. Non-admins must own the listing: team-domain `GetListing` (as the caller) must return `seller_id` equal to the caller id. Foreign listing is `PermissionDenied`, unknown listing `InvalidArgument`, lookup failure or `UPSTREAM_DOMAIN_ADDR` unset is `Unavailable` (fails closed). Admins skip the lookup. |
| `FlashSaleService/GetActiveFlashSale`, `ListActiveCampaigns`, `GetFlashSaleStock` | None (public reads). |
| `FlashSaleService/ReserveFlashSaleStock`, `CommitFlashSaleStock`, `ReleaseFlashSaleStock` | SERVICE principal holding `promotion.reserve` only (user/admin tokens are `PermissionDenied`); see [Flash stock holds](#flash-stock-holds). |
| `SubscriptionService/ListPlans` | None. |
| `SubscriptionService/Subscribe` | Requires `listing.write` (buyer-only is `PermissionDenied`); seller id is the caller's id. |
| `SubscriptionService/GetEntitlements` | Principal required. Only a `service` principal may read another seller; every other type (user, admin, unspecified) reads only its own id (fail closed). |
| `SponsoredService/CreateAdCampaign` | Requires `listing.write`; non-admins must own the listing (same team-domain check as `CreateCampaign`, fails closed). `bid`/`budget` above `MAX_AD_BID`/`MAX_AD_BUDGET` are `InvalidArgument`. Seller id is the caller's id. |
| `SponsoredService/ListSponsoredSlots` | None (active campaigns, best bid first). |

Also served: `grpc.health.v1.Health` (services `liveness` and `postgres`) and gRPC reflection (`GRPC_REFLECTION_ENABLED`, default false, only allowed when `ENV` is `local` or `test`).

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

`ValidateAndReserveResponse.scope` and `seller_id` echo the voucher's scope and owning seller on the valid, rejected and idempotent-replay paths. `scope` is `VOUCHER_SCOPE_SHOP` with `seller_id` set for shop vouchers, and the stored scope (`VOUCHER_SCOPE_PLATFORM`, or unspecified if it was created without one) with an empty `seller_id` for platform vouchers. Both are empty (unspecified scope, empty `seller_id`) when the code matches no voucher, or when a replayed `reservation_id` has no stored voucher.

## Layout

ADR-0016 layout under `internal/`:

| Package | Role |
|---|---|
| `internal/app` | wiring: `grpcx.NewServer`, `rpc.Register`, outbox relay, `migrate up` |
| `internal/config` | settings (SDK `config.Load`) |
| `internal/store` | Postgres stores and embedded migrations |
| `internal/events` | event envelopes written to the outbox |
| `internal/upstream` | team-audit and team-domain clients |
| `internal/listing` | listing ownership check |
| `internal/voucher` | voucher rules and discount arithmetic |
| `internal/flashsale` | flash-sale campaigns and stock holds |
| `internal/subscription` | plans and entitlements |
| `internal/sponsored` | sponsored ad campaigns |
| `internal/rpc` | gRPC handlers and authorization |

## Events

Producer only; nothing is consumed.

| Topic | Type (`EventEnvelope.type`) | Key | Emitted when |
|---|---|---|---|
| `promotion.events` (`PROMOTION_EVENTS_TOPIC`) | `platform.promotion.v1.VoucherChanged` | voucher id | `CreateVoucher` succeeds |
| `promotion.events` | `platform.promotion.v1.FlashSaleChanged` | campaign id | `CreateCampaign` succeeds |

Value is a proto-marshalled `platform.events.v1.EventEnvelope` (event id, type, principal,
`traceparent`, `request_id`, payload). Events are written to the `outbox` table in the same transaction as the change and relayed to Kafka
by the SDK outbox relay, so a Kafka outage delays publication but never fails the RPC. See ADR-0002.

## Data

Own database `promotion_db` (user `promotion_svc`). Migrations in `migrations/` (golang-migrate,
`.up.sql`/`.down.sql`):

| Migration | Tables |
|---|---|
| `0001_promotion` | `vouchers`, `voucher_reservations` (unique `reservation_id`), `flash_sale_campaigns` |
| `0002_subscriptions` | `subscription_plans` (seeded `plan_free`, `plan_pro`, `plan_premium`), `seller_subscriptions` (unique `seller_id`) |
| `0003_sponsored` | `ad_campaigns` (index on `(status, bid DESC)`) |
| `0004_flash_sale_holds` | Flash-sale stock holds (`reserve`, `commit`, `release`; see Flash stock holds) |
| `0005_outbox` | `outbox` (transactional event outbox) |

Migrations are embedded in `internal/store/migrations` and applied by `server migrate up` (the
`team-promotion-migrate` one-shot container in the root compose runs it; it needs only `ENV` and
`DATABASE_URL`). `0005_outbox` adds the `outbox` table used by the relay. The server itself does not
run migrations.

## Configuration

Read in `internal/config/config.go` through the SDK `config.Load` (struct tags are the source of truth).
Startup fails when a required key is missing or invalid.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | none | required |
| `LOG_LEVEL` | `info` | |
| `LOG_JSON` | `true` | JSON vs text slog |
| `GRPC_HOST` | `0.0.0.0` | |
| `GRPC_PORT` | none | required (the image sets `50061`) |
| `GRPC_REFLECTION_ENABLED` | `false` | refused unless `ENV` is `local` or `test` |
| `SHUTDOWN_GRACE` | `10s` | graceful stop deadline |
| `SHUTDOWN_DRAIN_DELAY` | `0s` | delay before stopping after readiness fails |
| `DATABASE_URL` | none | required |
| `DB_MAX_CONNS` | `10` | pgx pool size |
| `OTEL_ENABLED` | `false` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | |
| `OTEL_SERVICE_NAME` | `team-promotion` | |
| `FLIPT_ADDR` | `localhost:9000` | Flipt gRPC endpoint |
| `FEATURE_FLAGS_EVAL_TIMEOUT` | `500ms` | |
| `KAFKA_BROKERS` | `localhost:9092` | comma-separated; required in staging and prod |
| `PROMOTION_EVENTS_TOPIC` | `promotion.events` | |
| `OUTBOX_POLL_INTERVAL` | SDK default | outbox relay poll interval |
| `OUTBOX_BATCH_SIZE` | SDK default | rows per relay batch |
| `OUTBOX_CLAIM_LOCK` | SDK default | relay row claim lock duration |
| `UPSTREAM_AUDIT_ADDR` | none | required. team-audit gRPC address (`team-audit-svc:50066` in compose). Platform/unspecified-scope `CreateVoucher` (`voucher.admin_create`), admin `CreateCampaign` and admin `CreateAdCampaign` write an audit event (as `service-team-promotion`, `audit.write`) before inserting; an audit failure is `Unavailable` and nothing is created. Seller paths are not audited |
| `MAX_AD_BID` | `1000000` | Upper bound on a sponsored ad bid; larger is `InvalidArgument` |
| `MAX_AD_BUDGET` | `1000000000` | Upper bound on an ad campaign budget; larger is `InvalidArgument` |
| `UPSTREAM_DOMAIN_ADDR` | none | required. team-domain gRPC address (`team-domain-svc:50051` in compose) |

The only flag is `flash-sale-enabled` (fail-open, default true). Switched off, it makes
`GetActiveFlashSale` and `ListActiveCampaigns` return nothing.

## Run locally

Root compose (from the repo root; needs the `platform-core` infra up for `postgres`, `redpanda`
and `flipt` on the external `platform-core_default` network):

```bash
docker compose -f docker-compose.services.yaml up --build team-promotion
```

This runs `team-promotion-migrate` first, then the server on `:50061` with `DATABASE_URL` pointing
at the shared `postgres:5432`. There is no `--profile jobs` component.

Standalone (from `team-promotion/`):

```bash
cp .env.example .env     # DATABASE_URL points at localhost:5438
make proto               # generated/ is gitignored; needs buf
go run ./cmd/server migrate up   # needs a reachable Postgres
make run                 # go run ./cmd/server
```

Smoke test (grpcurl; set `GRPC_REFLECTION_ENABLED=true` locally):

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
| `make check` | merge gate: `gofmt`, `go vet ./...`, golangci-lint with the SDK config, race tests with `GO_SDK_REQUIRE_DOCKER=1` |
| `make test` | `go test ./...` |

Database tests run against Docker Postgres and never skip; a skip is a gate failure. This repo has no
`.github/workflows`; `make check` is the gate to run before merging.

## Spec and verification

- `FEATURES.yaml` declares 85 features: 32 `automated` (each with a `covered_by` scenario, mostly in
  `platform-e2e/tests/e2e/features/promo/` and `security/promotion_access.feature`) and 53 `not-testable`.
- Verify coverage with `make -C platform-e2e features-check` and, per change,
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Behaviour changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored (`buf generate`, ADR-0001). A fresh checkout does not compile until
  `make proto` has run. The Dockerfile does not run `buf`, so image builds need `generated/` present.
- `proto/` is vendored from `platform-core`. Never edit it here; change `platform-core/proto` and
  re-vendor.
- `GRPC_PORT` has no code default; the Dockerfile sets `GRPC_PORT=50061`.

## Flash stock holds

`FlashSaleService` exposes `ReserveFlashSaleStock`, `CommitFlashSaleStock` and
`ReleaseFlashSaleStock` for the order saga. They are service-only: the caller must be a
`service` principal with the `promotion.reserve` scope (users, even with the scope, are
`PERMISSION_DENIED`).

- Reserve holds units under a lock on the campaign row; a hold expires 15 minutes after reserve.
  It is idempotent on `reservation_id` and refuses with `not_active`, `price_changed` or
  `insufficient_stock` (checked in that order).
- Commit adds the held quantity to `stock_sold` (idempotent; refused once released or expired).
- Release returns the units, also after commit (it then decrements `stock_sold` once).
- `GetFlashSaleStock` reports `remaining = stock_cap - stock_sold - active holds`, floored at 0.
- Released and expired holds older than 30 days are purged daily; committed holds are kept.

## Known gaps

- **`ListVouchers` and `GetVoucher` are public**, including by arbitrary `seller_id`.
- **Quota counts commits only.** Outstanding (`reserved`) holds are not counted against `quota`, so
  concurrent reservations can exceed it before commits land.
- **No event on redemption or release.** `VoucherChanged` fires only on create, not when `used` changes.
- The `flash-sale-enabled` kill-switch only gates reads; it does not block `CreateCampaign`.

## Links

- Rules: [`../AGENTS.md`](../AGENTS.md); root ASDLC: [`../README.md`](../README.md)
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 auth model,
  0004 observability, 0008 inventory reservation model (mirrored by the reservation pattern),
  0010 service zero-trust
