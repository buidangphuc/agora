# team-order

Go gRPC service that owns the **cart, order lifecycle, purchase saga, shipments and returns (RMA)** bounded context. It is a deployed service (compose: `team-order-svc`, gRPC `:50055`, no host port published) with its own Postgres database `order_db` (Rule 3, database per service). It reserves and releases stock in `team-domain`, reads addresses from `team-identity`, optionally holds vouchers in `team-promotion`, moves orders to PAID from `PaymentSettled` events, and publishes order facts through a transactional outbox.

Go 1.22 (`go.mod`; the Dockerfile builds with `golang:1.22`), module `github.com/buidangphuc/team-order`.

## Contract

Served from `proto/platform/order/v1/order.proto` (vendored; see Gotchas). The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes` (`internal/interceptor/auth.go`). `RequirePrincipal` rejects missing, `anonymous` and ANONYMOUS-type principals. "Owner or admin" below means `isAdminOrUser` (`internal/handler/order.go`): the principal id equals the named user, or its scopes include `admin`, `order.admin` or `all`. This service does not check `order.read` / `order.write` scopes.

### CartService (`internal/handler/cart.go`)

| RPC | Authorization |
|---|---|
| `GetCart`, `ClearCart` | authenticated; acts on the caller's own cart |
| `AddToCart` | authenticated; price, title and seller are read from the listing via team-domain `GetListing` |
| `UpdateCartItem`, `RemoveFromCart` | authenticated; scoped to the caller's cart |
| `Reorder` | authenticated; caller must be the buyer of the source order (else `PermissionDenied`) |

### OrderService (`internal/handler/order.go`)

| RPC | Authorization / behaviour |
|---|---|
| `CreateOrder` | authenticated. Evaluates Flipt flag `checkout-enabled` first (off -> `FailedPrecondition`). Runs the purchase saga below. Returns one order per seller. |
| `GetOrder` | Principal required (`Unauthenticated` otherwise). Allowed: the order's buyer or seller, an admin scope, or a `service` principal with `order.read` (used by team-payment and team-engagement); others get `PermissionDenied`. |
| `ListBuyerOrders`, `ListSellerOrders` | authenticated; lists the caller's orders, optional status filter |
| `UpdateOrderStatus` | `RequirePrincipal`; the order's seller or admin only (`PermissionDenied` otherwise; buyers cancel via `CancelOrder`). Only PENDING or PAID -> SHIPPED and SHIPPED -> COMPLETED are allowed (`sellerTransitions` in `service/order.go`); any other transition, including to PAID or CANCELLED, returns `FailedPrecondition`. |
| `CancelOrder` | authenticated; buyer only. Releases stock per item, then sets CANCELLED. Rejects orders already CANCELLED or COMPLETED. |
| `CalculateShippingFee` | none. Free at subtotal >= 500000; 20000 for HCM / Ha Noi city strings; otherwise 35000 (`service/order.go`). |
| `GetSagaState` | owner or admin (buyer). The response is **derived from the order status**, not read from the saga tables. |
| `ForceFailSaga` | owner or admin (buyer). Cancels the order via `CancelOrder` and returns the derived saga state. `fail_step` is ignored. |
| `CreateReturnRequest` | authenticated; buyer of the order; order must not be PENDING or CANCELLED; `refund_amount` defaults to the order total and may not exceed it |
| `GetReturnRequest` | buyer or seller of the return, or admin |
| `UpdateReturnStatus` | seller of the return or admin. PENDING -> APPROVED/REJECTED; APPROVED -> REFUNDED/REJECTED; REJECTED and REFUNDED are terminal. |
| `CreateShipment` | seller of the order or admin. Defaults carrier to `SPX` and generates a tracking code. Also sets the order to SHIPPED. |
| `GetShipmentTracking` | none; by `tracking_code`, `order_id` or `shipment_id` |

The gRPC server also registers `grpc.health.v1` and, when `GRPC_REFLECTION_ENABLED=true`, reflection.

### Purchase saga (`CreateOrdersFromCart`: `internal/service/order.go`, `saga.go`, `redemption.go`)

1. Load the cart (optionally only `item_ids`); empty -> `FailedPrecondition`. Group items by `seller_id`.
2. Persist an `order_sagas` header, then per item persist an `order_reservations` row (stable id per buyer / cart item / quantity) **before** calling team-domain `ReserveStock`; mark it RESERVED.
3. If `voucher_code` is set and `UPSTREAM_PROMOTION_ADDR` is configured: `ValidateAndReserve` on team-promotion, once per checkout, on the first seller-order. The order id is the reservation id. A rejected voucher aborts with `FailedPrecondition`.
4. Create the PENDING order; mark its reservations COMMITTED.
5. Any failure runs compensation on a fresh background context: `ReleaseStock` with retries (3 attempts), release parked as RELEASE_FAILED on repeated failure, voucher hold released, saga marked COMPENSATED.
6. Payment is **not** a synchronous saga step. The order moves PENDING -> PAID when the `PaymentSettled` event arrives (see Events).

A background sweeper (only with Postgres) runs every `RESERVATION_SWEEP_INTERVAL` (default 1m) and releases reservations older than `RESERVATION_TTL` (default 15m) that were never bound to an order. Both are logged at sweeper start (`reservation sweeper starting`).

### Consumes (upstream)

| Upstream | Calls | Used by |
|---|---|---|
| team-domain (`UPSTREAM_DOMAIN_ADDR`) | `GetListing`, `ReserveStock`, `ReleaseStock` | cart add, checkout, cancel, compensation |
| team-identity (`UPSTREAM_IDENTITY_ADDR`) | `ListAddresses` | `CreateOrder` shipping address (matched by `address_id`, else default, else first; lookup errors are ignored and the address stays empty) |
| team-promotion (`UPSTREAM_PROMOTION_ADDR`, optional) | `ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` | voucher hold, commit on settle, release on saga failure |

`ReserveStock` and `ReleaseStock` always go to team-domain as the service principal `service-team-order` (type `service`, scopes `listing.read,listing.write,identity.read,identity.write`), never the end user's principal. Other upstream calls forward the incoming principal metadata (plus `listing.read,identity.read` scopes); with none they use the same service principal (`internal/upstream/domain.go`).

## Events

| Direction | Topic | Type (`EventEnvelope.type`) | Key | Trigger |
|---|---|---|---|---|
| consume | `payment.events` (`PAYMENT_EVENTS_TOPIC`) | `platform.payment.v1.PaymentSettled` | n/a | Sets a PENDING order to PAID when the payment status is PAID; commits the voucher hold if the order has one. Other statuses are ignored (the order is not cancelled). |
| produce | `order.events` (`ORDER_EVENTS_TOPIC`) | `platform.order.v1.OrderPaidEvent` | `order_id` | Outbox row written in the same transaction as the first transition to PAID |
| produce | `order.events` | `platform.order.v1.OrderShipped` | `order_id` | Outbox row written in the same transaction as `CreateShipment` |
| produce | `payment.events.dlq` (`PAYMENT_EVENTS_DLQ_TOPIC`) | original record | original key | Poison records, or records that exhausted retries |

- The payment consumer commits offsets only after a record is applied or dead-lettered (auto-commit disabled), retries up to 5 times in process, dedupes on `processed_events` (consumer `team-order.payment`), and runs only with Postgres and `KAFKA_ENABLED=true`.
- The outbox relayer claims rows with a lease, publishes the stored envelope verbatim, retries with backoff (1s doubling to 5m), and parks a row (`status='failed'`) after `OUTBOX_MAX_ATTEMPTS`. It runs only when Postgres, `KAFKA_ENABLED` and `OUTBOX_ENABLED` are all true.
- With `KAFKA_ENABLED=false` (the default) outbox rows are still written but never published, and orders do not move to PAID from payment events.
- Event ids are deterministic (per order for paid, per shipment for shipped). Consumers in this workspace include team-analytics (`OrderPaidEvent`) and team-notification (`order.events`).

## Data

Postgres `order_db` (compose credentials `order_svc` / `order_pass`, shared Postgres host `postgres:5432`). Migrations are in `migrations/` as golang-migrate pairs (`.up.sql` / `.down.sql`).

| Migration | Tables / changes |
|---|---|
| 0001 | `cart_items`, `orders`, `order_items` |
| 0002 | `orders.shipping_fee`, `items_subtotal`, `payment_method` |
| 0003 | `order_returns`, `shipments`, `shipment_checkpoints` |
| 0004 | `order_sagas`, `order_reservations`, `processed_events` |
| 0005 | `orders.voucher_code`, `discount_amount` |
| 0006 | `order_outbox_events` (status `pending` / `published` / `failed`) |

The service does **not** migrate on boot. Compose applies migrations with the one-shot `team-order-migrate` job (`migrate/migrate:v4.17.1`, `up`, idempotent), which `team-order` depends on. Standalone, run golang-migrate against `migrations/` yourself.

With `DATABASE_ENABLED=false` every repository and the saga store fall back to in-memory implementations, and the Kafka consumer, relayer and sweeper do not start.

## Configuration

Read by `internal/config/config.go` (struct tags are the source of truth). Defaults below are the code defaults, not `.env.example` values.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `local` | Only feeds `IsProd()`, which nothing calls |
| `LOG_LEVEL` | `info` | Only `debug` changes behaviour; anything else is info |
| `LOG_JSON` | `true` | |
| `GRPC_HOST` / `GRPC_PORT` | `0.0.0.0` / `50055` | |
| `GRPC_REFLECTION_ENABLED` | `true` | |
| `SHUTDOWN_GRACE_SECONDS` | `10` | |
| `DATABASE_ENABLED` | `true` | |
| `DATABASE_URL` | empty | Required when `DATABASE_ENABLED=true` (startup fails otherwise) |
| `DB_MAX_CONNS` | `10` | |
| `UPSTREAM_DOMAIN_ADDR` | `localhost:50051` | |
| `UPSTREAM_IDENTITY_ADDR` | `localhost:50053` | |
| `UPSTREAM_PROMOTION_ADDR` | empty | Empty disables vouchers; `voucher_code` is then ignored |
| `OTEL_ENABLED` | `false` | Declared but not read (see Known gaps) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Declared but not read |
| `OTEL_SERVICE_NAME` | `team-order` | Declared but not read |
| `FEATURE_FLAGS_ENABLED` | `true` | `false` makes every flag return its default |
| `FLIPT_ADDR` | `localhost:9000` | Flipt gRPC address |
| `FEATURE_FLAGS_EVAL_TIMEOUT_MS` | `500` | |
| `KAFKA_ENABLED` | `false` | Gates the payment consumer and the outbox relayer |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated |
| `ORDER_PAYMENT_CONSUMER_GROUP` | `team-order.payment` | |
| `PAYMENT_EVENTS_TOPIC` | `payment.events` | |
| `PAYMENT_EVENTS_DLQ_TOPIC` | `payment.events.dlq` | |
| `ORDER_EVENTS_TOPIC` | `order.events` | |
| `OUTBOX_ENABLED` | `true` | Relayer also needs `KAFKA_ENABLED=true` |
| `OUTBOX_POLL_INTERVAL` | `1s` | Go duration; invalid or non-positive falls back to 1s |
| `OUTBOX_BATCH_SIZE` | `100` | |
| `OUTBOX_CLAIM_LOCK_SECONDS` | `60` | |
| `OUTBOX_MAX_ATTEMPTS` | `10` | |
| `RESERVATION_TTL` | `15m` | Go duration: stock-hold lifetime before the sweep releases it; also how long an unfinished checkout attempt may stay pending. Empty, invalid or non-positive falls back to the default with a WARN; boot never fails |
| `RESERVATION_SWEEP_INTERVAL` | `1m` | Go duration: sweep cadence. Same fallback rule |

Drift gate: `TestEnvExampleInSync` (`internal/config/config_test.go`) fails if `.env.example` and the declared env keys differ in either direction. `.env.example` points `DATABASE_URL` at `localhost:5437`; compose uses `postgres:5432`.

### Feature flag: checkout kill-switch

Boolean flag `checkout-enabled` (`internal/featureflags/featureflags.go`), evaluated through OpenFeature with the Flipt provider at the start of `CreateOrder`. It **fails open**: if Flipt is disabled, unreachable at boot, errors, or exceeds `FEATURE_FLAGS_EVAL_TIMEOUT_MS`, checkout stays enabled. Only an explicit OFF in Flipt returns `FailedPrecondition: checkout is temporarily unavailable`. Flip it in the Flipt UI; no redeploy is needed. `go.mod` and `internal/featureflags/provider.go` still mark the Flipt provider package and its streaming behaviour as ASSUMED, to be confirmed.

## Run locally

Root compose (preferred), from the repo root:

```bash
docker compose up -d --build            # whole stack: shared infra + services + migrate jobs
docker compose up -d --build team-order # this service and its dependencies (team-order-migrate, redpanda-init, Postgres)
```

`team-order` is wired in `docker-compose.services.yaml` with Kafka and the outbox relayer enabled, Flipt at `flipt:9000`, promotion at `team-promotion-svc:50061`, and topics created by `redpanda-init`. Its gRPC port is reachable only on the compose network as `team-order-svc:50055`; use the gateway from the host.

Standalone (no compose): apply `migrations/` to a Postgres, generate code (below), then:

```bash
set -a; . ./.env; set +a   # from .env.example; the process reads the environment and does not load .env itself
go run ./cmd/server
```

With `DATABASE_ENABLED=false` it runs entirely in memory (no PAID transitions from Kafka, no outbox).

## Build, test and lint

```bash
buf generate                # writes generated/ using ./buf.gen.yaml (input: ./proto)
go build ./cmd/server
go vet ./...
gofmt -l .                  # should list nothing outside generated/
go test ./...
docker build -t team-order:local .
```

There is no Makefile, no `.github/workflows` and no other CI config in this repository; run the commands above as the gate. Tests named `*_pg_test.go` run only when `TEST_DATABASE_URL` (or `DATABASE_URL`) points at a migrated Postgres, otherwise they skip.

## Spec and verification

- `FEATURES.yaml` (this repo) lists the user-facing features and their e2e coverage in `platform-e2e`.
- Verify from the repo root: `make -C platform-e2e features-check` (manifests and coverage) and `make -C platform-e2e spec-check CHANGE=<id>` (every scenario of an OpenSpec change is green).
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root): propose, implement code and e2e together, verify, archive. See the root `README.md` (ASDLC).

## Gotchas

- `generated/` is gitignored (root `.gitignore`) and must be produced with `buf generate`. The Dockerfile builds from the local directory, so the build fails if `generated/` is absent. Never hand-edit it.
- `proto/` is vendored from `platform-core/packages/proto`. Never edit it here; change the contract in platform-core. `proto/buf.gen.yaml` is platform-core's file; use the root `buf.gen.yaml`, which rewrites `go_package` to this module.
- In-memory fallbacks: `DATABASE_ENABLED=false` (everything in memory), no `UPSTREAM_PROMOTION_ADDR` (no vouchers), Flipt unavailable (flags default, checkout open).
- `KAFKA_ENABLED` defaults to `false`: orders stay PENDING after payment and outbox rows accumulate as `pending`.
- `coverage.out` and `coverage.summary` in the directory are stray test artifacts, not part of the build.

## Known gaps

- `CalculateShippingFee` and `GetShipmentTracking` are unauthenticated.
- Scopes `order.read` / `order.write` are not enforced here; authorization is only the id and admin-scope checks above. Principal metadata is trusted as forwarded.
- `GetSagaState` is synthesized from the order status with fixed text and timestamps; it does not read `order_sagas` or `order_reservations`. `ForceFailSaga` ignores `fail_step` and cancels any order that is not already CANCELLED or COMPLETED.
- `CancelOrder` allows cancelling PAID and SHIPPED orders, only logs `ReleaseStock` errors, and does not release a voucher hold.
- A failed `PaymentSettled` leaves the order PENDING; nothing cancels it automatically.
- `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` and `ENV` are inert: only the otelgrpc stats handler is attached, and no tracer provider or exporter is configured in this repo.
- `CreateShipment` does not check order status.
- `FEATURES.yaml` references `order.v1.ListOrders`, which does not exist (the RPCs are `ListBuyerOrders` and `ListSellerOrders`).

## Links

- Rules and recipe: root `AGENTS.md`.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 / 0006 / 0010 auth and zero trust, 0007 durable saga and compensation, 0008 inventory reservation model, 0009 payment-order event integration, 0013 seller demand forecasting and order warehouse integration (order events).
