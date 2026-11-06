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
| `VoucherService/CreateVoucher` | Principal required. Scope `PLATFORM` (and unspecified) needs `admin`. Scope `SHOP` needs `listing.write` or `admin`; `seller_id` is forced to the caller's id. `per_user_limit` outside 0..1000 is `InvalidArgument`; the voucher is always `VOUCHER_KIND_STANDARD` with no owner. |
| `VoucherService/GetVoucher` | None (public read by code). A user-bound voucher (`owner_user_id` set) is readable only by its owner; anyone else, anonymous included, gets `NotFound`. |
| `VoucherService/ListVouchers` | None. Filters by `seller_id` from the request (empty = platform). User-bound vouchers are never listed. |
| `VoucherService/ListMyVouchers` | Any signed-in principal, no scope; `Unauthenticated` for anonymous. The caller's own usable vouchers (active, started, not expired, not used up), soonest expiry first, at most 50. |
| `VoucherService/DeactivateVoucher` | Scope `admin`. Writes a `voucher.deactivate` audit event first (fail-closed, `Unavailable` with `ErrorInfo` `promotion.platform/AUDIT_UNAVAILABLE`), then sets `active = false`; idempotent (keeps the first `deactivated_at`). An inactive voucher is refused by `ValidateAndReserve` (`internal/voucher/voucher.go:350`). |
| `VoucherService/UpdateVoucher` | Scope `admin`. Only `ends_at` (must be in the future and after `starts_at`), `quota` (positive, never lowered, not on an unlimited voucher), `per_user_limit` (0..1000) and `new_user_only` change; the last two are `InvalidArgument` on a `VOUCHER_KIND_NEW_USER` voucher, and `owner_user_id` and `kind` are server-owned; nothing to change is `InvalidArgument`, an inactive voucher `FailedPrecondition`. Audited as `voucher.update` (fail-closed) (`internal/voucher/voucher.go:173`). |
| `VoucherService/ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` | `Commit`/`Release`: SERVICE principal holding `promotion.reserve` only (user/admin tokens are `PermissionDenied`). `ValidateAndReserve` is dual-mode: a SERVICE principal with `promotion.reserve` is trusted (request `buyer_id`/`seller_id` honoured); any other authenticated principal gets `buyer_id` forced to its own id and `reservation_id` must start `preview:<its id>:` (else `PermissionDenied`), so a preview hold can never be committed. |
| `FlashSaleService/CreateCampaign` | Principal required plus `listing.write` or `admin`. Non-admins must own the listing: team-domain `GetListing` (as the caller) must return `seller_id` equal to the caller id. Foreign listing is `PermissionDenied`, unknown listing `InvalidArgument`, lookup failure or `UPSTREAM_DOMAIN_ADDR` unset is `Unavailable` (fails closed, `ErrorInfo` `promotion.platform/LISTING_UNAVAILABLE`). `sale_price` and `stock_cap` must be positive, `ends_at` after `starts_at` and in the future, and `sale_price` below the listing price. Admins must send an `idempotency_key` (8-64 of `[A-Za-z0-9_-]`): a replay with the same payload returns the first campaign, a different payload is `AlreadyExists`; the admin path now also looks the listing up (no owner check) (`internal/flashsale/flashsale.go`, `internal/rpc/flashsale.go`). |
| `FlashSaleService/GetActiveFlashSale`, `ListActiveCampaigns`, `GetFlashSaleStock` | None (public reads). `ListActiveCampaigns` sets `flash_sale_disabled` when the kill-switch is off. |
| `FlashSaleService/ListCampaigns` | Scope `admin`. Every campaign, newest first, optional `statuses` filter (`SCHEDULED`, `ACTIVE`, `ENDED`, `CANCELLED`, derived from the window and `cancelled_at`), offset cursor; also returns `flash_sale_disabled` (`internal/flashsale/flashsale_list.go:66`). |
| `FlashSaleService/CancelCampaign` | Scope `admin`. Scheduled or active only (ended is `FailedPrecondition`, unknown `NotFound`); audited as `campaign.cancel` (fail-closed); sets `cancelled_at`, after which reserve refuses the campaign (`internal/flashsale/flashsale_cancel.go:21`). |
| `FlashSaleService/UpdateCampaign` | Scope `admin`. Scheduled campaigns only (`FailedPrecondition` otherwise); same price, stock cap and window checks as create, price checked against the listing; audited as `campaign.update` (fail-closed) (`internal/flashsale/flashsale_update.go:33`). |
| `FlashSaleService/ReserveFlashSaleStock`, `CommitFlashSaleStock`, `ReleaseFlashSaleStock` | SERVICE principal holding `promotion.reserve` only (user/admin tokens are `PermissionDenied`); see [Flash stock holds](#flash-stock-holds). |
| `SubscriptionService/ListPlans` | None. |
| `SubscriptionService/Subscribe` | Requires `listing.write` (buyer-only is `PermissionDenied`); seller id is the caller's id. |
| `SubscriptionService/GetEntitlements` | Principal required. Only a `service` principal may read another seller; every other type (user, admin, unspecified) reads only its own id (fail closed). |
| `SponsoredService/CreateAdCampaign` | Requires `listing.write`; non-admins must own the listing (same team-domain check as `CreateCampaign`, fails closed). `bid`/`budget` above `MAX_AD_BID`/`MAX_AD_BUDGET` are `InvalidArgument`. Seller id is the caller's id. |
| `SponsoredService/ListSponsoredSlots` | None (active campaigns, best bid first). |

Also served: `grpc.health.v1.Health` (services `liveness` and `postgres`) and gRPC reflection (`GRPC_REFLECTION_ENABLED`, default false, only allowed when `ENV` is `local` or `test`).

**Consumes:** team-order `OrderService/HasCompletedOrder` (service principal `service-team-promotion`, scope `order.read`, `UPSTREAM_ORDER_ADDR`) for `new_user_only` vouchers; any error, timeout or refusal is `Unavailable` (fail closed). Kafka `identity.events` (`UserRegistered`) and `referral.events` (`ReferralAttributed`, `ReferralRejected`) issue one `VOUCHER_KIND_NEW_USER` voucher per account (see [New-user vouchers](#new-user-vouchers)). team-domain `ListingService/GetListing` (vendored `platform/listing/v1`), called with the
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

Per-user rules: after the idempotent-replay check, `ValidateAndReserve` refuses with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE` when the voucher has an owner other than the buyer, or when `new_user_only` is set and the buyer has a completed order; and with `VOUCHER_PER_USER_LIMIT` when `per_user_limit > 0` and the buyer already has that many committed or unexpired reserved holds. The per-user count and the hold insert share one transaction under an advisory lock per (voucher, buyer). These run before the global quota check. A hold expires `VOUCHER_RESERVATION_TTL` after it is created (`voucher_reservations.expires_at`) and then stops counting; committed holds always count. An unavailable `HasCompletedOrder` is gRPC `UNAVAILABLE`; no hold is made.

## New-user vouchers

`internal/consumer` consumes `identity.events` and `referral.events` (groups `team-promotion-identity` and `team-promotion-referral`, DLQ topics `identity.events.promotion.dlq` and `referral.events.promotion.dlq` after 3 attempts, malformed events are permanent failures). Each account gets at most one `VOUCHER_KIND_NEW_USER` voucher: a fixed, platform-scoped, single-quota, `per_user_limit` 1 voucher bound to the account, valid for `NEW_USER_VOUCHER_TTL` from issue. `UserRegistered` without a referral code issues the welcome voucher (`NEW_USER_WELCOME_AMOUNT`). With a code it waits for the referral verdict: `ReferralAttributed` issues the invited voucher (`NEW_USER_INVITED_AMOUNT`), `ReferralRejected` issues the welcome voucher, and a verdict that arrives before the registration is kept and used. If no verdict arrives within `NEW_USER_VERDICT_TIMEOUT` the sweeper (every `NEW_USER_VERDICT_SWEEP_INTERVAL`) issues the welcome voucher; a later verdict changes nothing. Processed event ids (`promotion_processed_events`) make redelivery a no-op; issued bookkeeping older than 30 days is purged daily. NEW_USER vouchers emit no `VoucherChanged`, and logs carry only fingerprints.

`ValidateAndReserveResponse.scope` and `seller_id` echo the voucher's scope and owning seller on the valid, rejected and idempotent-replay paths. `scope` is `VOUCHER_SCOPE_SHOP` with `seller_id` set for shop vouchers, and the stored scope (`VOUCHER_SCOPE_PLATFORM`, or unspecified if it was created without one) with an empty `seller_id` for platform vouchers. Both are empty (unspecified scope, empty `seller_id`) when the code matches no voucher, or when a replayed `reservation_id` has no stored voucher.

## Layout

ADR-0016 layout under `internal/`:

| Package | Role |
|---|---|
| `internal/app` | wiring: `grpcx.NewServer`, `rpc.Register`, outbox relay, `migrate up` |
| `internal/config` | settings (SDK `config.Load`) |
| `internal/store` | Postgres stores and embedded migrations |
| `internal/events` | event envelopes written to the outbox |
| `internal/upstream` | team-audit, team-domain and team-order clients |
| `internal/consumer` | new-user voucher issuance from identity and referral events, verdict sweeper, purge |
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

Own database `promotion_db` (user `promotion_svc`). Migrations in `internal/store/migrations/` (golang-migrate,
`.up.sql`/`.down.sql`):

| Migration | Tables |
|---|---|
| `0001_promotion` | `vouchers`, `voucher_reservations` (unique `reservation_id`), `flash_sale_campaigns` |
| `0002_subscriptions` | `subscription_plans` (seeded `plan_free`, `plan_pro`, `plan_premium`), `seller_subscriptions` (unique `seller_id`) |
| `0003_sponsored` | `ad_campaigns` (index on `(status, bid DESC)`) |
| `0004_flash_sale_holds` | Flash-sale stock holds (`reserve`, `commit`, `release`; see Flash stock holds) |
| `0005_outbox` | `outbox` (transactional event outbox) |
| `0006_voucher_active` | adds `vouchers.active` (default true), `deactivated_at` |
| `0007_campaign_cancel_idempotency` | adds `flash_sale_campaigns.cancelled_at`; `campaign_idempotency` (PK `actor_id`, `key`; payload hash, campaign id) |

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
| `UPSTREAM_AUDIT_ADDR` | none | required. team-audit gRPC address (`team-audit-svc:50066` in compose). Platform/unspecified-scope `CreateVoucher` (`voucher.admin_create`), `DeactivateVoucher`, `UpdateVoucher`, admin `CreateCampaign`, `CancelCampaign`, `UpdateCampaign` and admin `CreateAdCampaign` write an audit event (as `service-team-promotion`, `audit.write`) before inserting; an audit failure is `Unavailable` and nothing is created. Seller paths are not audited |
| `MAX_AD_BID` | `1000000` | Upper bound on a sponsored ad bid; larger is `InvalidArgument` |
| `MAX_AD_BUDGET` | `1000000000` | Upper bound on an ad campaign budget; larger is `InvalidArgument` |
| `UPSTREAM_DOMAIN_ADDR` | none | required. team-domain gRPC address (`team-domain-svc:50051` in compose) |
| `UPSTREAM_ORDER_ADDR` | none | team-order gRPC address (`team-order-svc:50055` in compose). Unset does not stop boot; `new_user_only` checks then fail closed with `Unavailable` |
| `UPSTREAM_ORDER_TIMEOUT` | `500ms` | per-call bound for `HasCompletedOrder` |
| `NEW_USER_WELCOME_AMOUNT` | `20000` | welcome voucher discount, must be > 0 |
| `NEW_USER_INVITED_AMOUNT` | `30000` | invited voucher discount, must be > 0 |
| `NEW_USER_MIN_SPEND` | `100000` | minimum subtotal of new-user vouchers, >= 0 |
| `NEW_USER_VOUCHER_TTL` | `336h` | validity of an issued new-user voucher, must be > 0 |
| `NEW_USER_VERDICT_TIMEOUT` | `30m` | wait for a referral verdict before issuing the welcome voucher |
| `NEW_USER_VERDICT_SWEEP_INTERVAL` | `1m` | sweeper period |
| `VOUCHER_RESERVATION_TTL` | `15m` | lifetime of a reserved hold for per-user counting |

The only flag is `flash-sale-enabled` (fail-open, default true). Switched off, it makes
`GetActiveFlashSale` and `ListActiveCampaigns` return nothing; `ListActiveCampaigns` and `ListCampaigns` report it in `flash_sale_disabled`.

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

- `FEATURES.yaml` declares 301 features: 130 `automated` (each with a `covered_by` scenario, mostly in
  `platform-e2e/tests/e2e/features/promo/` and `security/promotion_access.feature`), 169 `not-testable` and 2 `planned`.
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
- **No event on admin changes.** `DeactivateVoucher`, `UpdateVoucher`, `CancelCampaign` and `UpdateCampaign` write audit events but no `VoucherChanged`/`FlashSaleChanged` outbox row (`internal/voucher/voucher.go:150`, `internal/flashsale/flashsale_cancel.go:21`).
- The `flash-sale-enabled` kill-switch only gates reads; it does not block `CreateCampaign`.

## Links

- Rules: [`../AGENTS.md`](../AGENTS.md); root ASDLC: [`../README.md`](../README.md)
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 auth model,
  0004 observability, 0008 inventory reservation model (mirrored by the reservation pattern),
  0010 service zero-trust
