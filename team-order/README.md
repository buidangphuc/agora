# team-order

Go gRPC service that owns the **cart, order lifecycle, purchase saga, shipments and returns (RMA)** bounded context. It is a deployed service (compose: `team-order-svc`, gRPC `:50055`, no host port published) with its own Postgres database `order_db` (Rule 3, database per service). It reserves and releases stock in `team-domain`, reads addresses from `team-identity`, optionally holds vouchers in `team-promotion`, moves orders to PAID from `PaymentSettled` events, and publishes order facts through a transactional outbox.

Go 1.22 (`go.mod`; the Dockerfile builds with `golang:1.22`), module `github.com/buidangphuc/team-order`.

## Contract

Served from `proto/platform/order/v1/order.proto` (vendored; see Gotchas). The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes` (`internal/interceptor/auth.go`). `RequirePrincipal` rejects missing, `anonymous` and ANONYMOUS-type principals. "Owner or admin" below means `isAdminOrUser` (`internal/handler/order.go`): the principal id equals the named user, or its scopes include `order.admin` (the bare `admin` scope does not open orders). This service does not check `order.read` / `order.write` scopes.

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
| `CreateOrder` | authenticated. Evaluates Flipt flag `checkout-enabled` first (off -> `FailedPrecondition`). Optional `idempotency-key` metadata (see below). Runs the purchase saga below. Returns one order per seller, all or none. |
| `GetOrder` | Principal required (`Unauthenticated` otherwise). Allowed: the order's buyer or seller, an admin scope, or a `service` principal with `order.read` (used by team-payment and team-engagement); others get `PermissionDenied`. `Order.paid_at` is set when the order moved to PAID through a payment; unset means it was never paid online (e.g. cash on delivery). |
| `ListBuyerOrders`, `ListSellerOrders` | authenticated; lists the caller's orders, optional status filter |
| `UpdateOrderStatus` | `RequirePrincipal`; the order's seller or admin only (`PermissionDenied` otherwise; buyers cancel via `CancelOrder`). Follows the transition table below: a target the seller may never request (PAID, PENDING, CANCELLED) -> `PermissionDenied`; a permitted target from the wrong status -> `FailedPrecondition`. |
| `CancelOrder` | authenticated; buyer only. Claims CANCELLED from PENDING or PAID (else `FailedPrecondition`, nothing released), then releases the order's reservations by id and its voucher hold. |
| `CalculateShippingFee` | none. Free at subtotal >= 500000; 20000 for HCM / Ha Noi city strings; otherwise 35000 (`service/order.go`). |
| `GetSagaState` | owner or admin (buyer). Built from the order, its reservations and `paid_at` (see Saga view). |
| `ForceFailSaga` | admin only: scopes `admin` AND `order.admin` (owners get `PermissionDenied`). `fail_step` must be empty, `payment` or `shipping` (`InvalidArgument`). Cancels via `CancelOrder`; `success=false` when a stock release is parked. |
| `CreateReturnRequest` | authenticated; buyer of the order; order must not be PENDING or CANCELLED. **Return cap:** `refund_amount` may not exceed the order's returnable remainder, the order total minus the `refund_amount` of the order's returns that are not REJECTED (`InvalidArgument`, also when the remainder is 0). No amount (`<= 0`) defaults to the remainder; no amount with a remainder of 0 is `FailedPrecondition`. The check and the insert run under the order row lock (`SELECT ... FOR UPDATE`), so concurrent requests never exceed the total; a rejected return frees its amount. |
| `GetReturnRequest` | buyer or seller of the return, or admin |
| `ListOrderReturns` | buyer or seller of the order, or admin (else `PermissionDenied`; unknown order `NotFound`). All returns of the order, newest first. |
| `UpdateReturnStatus` | seller of the return or admin (the buyer gets `PermissionDenied`). PENDING -> APPROVED/REJECTED; APPROVED -> REFUNDED/REJECTED; REJECTED and REFUNDED are terminal (`FailedPrecondition`). Every transition is a compare-and-set on the status read (`TransitionReturn`): of concurrent transitions one wins, the others get `FailedPrecondition`. The won APPROVED -> REFUNDED writes `ReturnRefunded` to the outbox in the same transaction. A move to REFUNDED on an order with no `paid_at` (never paid online, e.g. cash on delivery) is refused with `FailedPrecondition: order was not paid online; cash-on-delivery refunds are handled outside the system`, keeps the return's status and writes nothing; approve and reject still work. team-order never calls team-payment to refund. |
| `CreateShipment` | seller of the order or admin. Claims SHIPPED from PENDING or PAID first (else `FailedPrecondition`, no shipment), then creates the shipment. Defaults carrier to `SPX` and generates a tracking code. |
| `GetShipmentTracking` | none; by `tracking_code`, `order_id` or `shipment_id` |

The gRPC server also registers `grpc.health.v1` and, when `GRPC_REFLECTION_ENABLED=true`, reflection.

### Purchase saga (`CreateOrdersFromCart`: `internal/service/order.go`, `saga.go`, `idempotency.go`, `redemption.go`)

A checkout places **every order or none** (design D6 of `port-order-inventory-correctness`):

1. Load the cart (optionally only `item_ids`); empty -> `FailedPrecondition`; buying your own listing -> `FailedPrecondition`. Group items by `seller_id`, **sorted**, one pre-generated order id per group.
2. Persist an `order_sagas` header (one per attempt). Reservation ids are `sha1(saga_id | cart item | listing | variant | qty)`: every attempt holds its own reservations.
3. **Phase A, reserve:** per item persist an `order_reservations` row (PENDING) **before** team-domain `ReserveStock`, then RESERVED. Short stock -> `ResourceExhausted`. If `voucher_code` is set and `UPSTREAM_PROMOTION_ADDR` is configured: `ValidateAndReserve` once, on the first group; the hold id is that order's id. A rejected voucher -> `FailedPrecondition`.
4. **Phase B, commit:** `CommitReservation` for every hold in team-domain (its TTL sweep then never restores them). A hold already released -> `FailedPrecondition` ("item no longer reserved"); any other commit error fails the checkout too.
5. **Phase C, place:** one `order_db` transaction inserts every order and its items, binds each reservation RESERVED -> COMMITTED with its `order_id`, and marks the saga COMPLETED (`OrderPlacer`, `internal/repository/placer.go`).
6. Any failure in A/B, or a definite failure in C, compensates on a fresh context: every held reservation released by id (3 attempts; a failure is parked RELEASE_FAILED for the sweep), the voucher hold released, saga COMPENSATED. An ambiguous C error is reconciled by looking the order ids up: all present -> placed; none -> compensate; partial -> `Internal`, nothing released.
7. The cart is cleared after C (best effort).
8. Payment is **not** a synchronous step. The order moves PENDING -> PAID when `PaymentSettled` arrives (see Events).

**Idempotency.** `CreateOrder` reads the `idempotency-key` metadata (the gateway forwards `Idempotency-Key`), trimmed, 1-255 printable ASCII bytes, else `InvalidArgument` before anything is reserved. Keys are unique per buyer on `order_sagas` (`(buyer_id, idempotency_key)` partial unique index). A repeat of a COMPLETED checkout returns its orders and retries the cart clear; one still running -> `Aborted` (retry); a compensated checkout frees its key. No key = a fresh checkout every time.

**Status changes** follow one table (`internal/service/transitions.go`) written with a compare-and-set (`UpdateOrderStatusFrom`: `UPDATE ... WHERE status = ANY(allowed)`):

| From | To | Who |
|---|---|---|
| Pending | Paid | payment consumer only (records `paid_at`) |
| Pending | Shipped | seller (COD hand-over), via `UpdateOrderStatus` or `CreateShipment` |
| Paid | Shipped | seller |
| Shipped | Completed | seller |
| Pending, Paid | Cancelled | buyer (`CancelOrder`), admin (`ForceFailSaga`) |

An admin acts as the seller on `UpdateOrderStatus`. A target the caller may never request -> `PermissionDenied`; a permitted target from the wrong status -> `FailedPrecondition`. `CancelOrder` claims Cancelled first; only the winner releases the order's own reservations (by id) and its voucher hold. `CreateShipment` claims Shipped first and creates the shipment only on a won claim. The `orders_status_check` constraint rejects any status outside 1..5.

**Sweep.** Only with Postgres, every `RESERVATION_SWEEP_INTERVAL` (default 1m): releases reservations older than `RESERVATION_TTL` (default 15m) that no order owns, releases holds still on Cancelled orders (a crash between a cancel's claim and its release, or a parked release), and compensates checkout attempts still PENDING a full TTL after they started (freeing their key). Both values are logged at sweeper start (`reservation sweeper starting`).

**Saga view.** `GetSagaState` is built from the order row, its reservations and `paid_at` (no invented times). `ForceFailSaga` accepts `fail_step` "", `payment` or `shipping`, cancels through `CancelOrder`, and answers `success=false` ("stock release is pending retry") when a release is parked.

### Consumes (upstream)

| Upstream | Calls | Used by |
|---|---|---|
| team-domain (`UPSTREAM_DOMAIN_ADDR`) | `GetListing`, `ReserveStock`, `CommitReservation`, `ReleaseStock` | cart add, checkout, cancel, compensation, sweep, `cmd/resync-commits` |
| team-identity (`UPSTREAM_IDENTITY_ADDR`) | `ListAddresses` | `CreateOrder` shipping address (matched by `address_id`, else default, else first; lookup errors are ignored and the address stays empty) |
| team-promotion (`UPSTREAM_PROMOTION_ADDR`, optional) | `ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` | voucher hold, commit on settle, release on saga failure |

`ReserveStock`, `CommitReservation` and `ReleaseStock` always go to team-domain as the service principal `service-team-order` (type `service`) with exactly the scope `listing.write`, never the end user's principal; voucher saga RPCs carry only `promotion.reserve`. Other upstream calls forward the incoming principal metadata unchanged; with none they use the same service principal with only `listing.read` (`internal/upstream/domain.go`).

## Events

| Direction | Topic | Type (`EventEnvelope.type`) | Key | Trigger |
|---|---|---|---|---|
| consume | `payment.events` (`PAYMENT_EVENTS_TOPIC`) | `platform.payment.v1.PaymentSettled` | n/a | Sets a PENDING order to PAID (compare-and-set, records `paid_at`) when the payment status is PAID; an order in any other status is left unchanged and logged. Commits the voucher hold only for an order that reached PAID. Other payment statuses are ignored. |
| produce | `order.events` (`ORDER_EVENTS_TOPIC`) | `platform.order.v1.OrderPaidEvent` | `order_id` | Outbox row written in the same transaction as the first transition to PAID |
| produce | `order.events` | `platform.order.v1.OrderShipped` | `order_id` | Outbox row written in the same transaction as `CreateShipment` |
| produce | `order.events` | `platform.order.v1.OrderCancelled` | `order_id` | Outbox row written in the same transaction as the compare-and-set claim to CANCELLED (`CancelOrder`, `ForceFailSaga`), only when the claim wins. `previous_status` is `ORDER_STATUS_PAID` when the order was cancelled from Paid (it has `paid_at`), else `ORDER_STATUS_PENDING` |
| produce | `order.events` | `platform.order.v1.ReturnRefunded` | `order_id` | Outbox row written in the same transaction as the won APPROVED -> REFUNDED compare-and-set of a return (`UpdateReturnStatus`); never for an order without `paid_at`. Carries the return, order, buyer and seller ids, the stored `refund_amount` (never a client value), the order currency and `refunded_at`. Consumed by team-payment, which refunds the order's payment once under refund id `return:<return_id>` (clamped to what the payment still has) |
| produce | `payment.events.dlq` (`PAYMENT_EVENTS_DLQ_TOPIC`) | original record | original key | Poison records, or records that exhausted retries |

- The payment consumer commits offsets only after a record is applied or dead-lettered (auto-commit disabled), retries up to 5 times in process, dedupes on `processed_events` (consumer `team-order.payment`), and runs only with Postgres and `KAFKA_ENABLED=true`.
- The outbox relayer claims rows with a lease, publishes the stored envelope verbatim, retries with backoff (1s doubling to 5m), and parks a row (`status='failed'`) after `OUTBOX_MAX_ATTEMPTS`. It runs only when Postgres, `KAFKA_ENABLED` and `OUTBOX_ENABLED` are all true.
- With `KAFKA_ENABLED=false` (the default) outbox rows are still written but never published, and orders do not move to PAID from payment events.
- Event ids are deterministic UUIDv5s (per order for paid and cancelled, per shipment for shipped, per return for `ReturnRefunded`, namespace `agora/team-order/order.events/<Type>`). Consumers in this workspace include team-analytics (`OrderPaidEvent`), team-notification (`order.events`) and team-payment (consumer group `team-payment.settlement`: `OrderPaidEvent` credits the seller, `OrderCancelled` with `previous_status = ORDER_STATUS_PAID` refunds what remains of the payment and deducts the seller, `ReturnRefunded` refunds the return). Consumers filter on `EventEnvelope.type`, so ones that do not handle `OrderCancelled` or `ReturnRefunded` ignore them.
- Deploy order: team-payment (which understands `ReturnRefunded`) must be deployed before this service emits it; an older team-payment ignores and commits the record, losing it.

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
| 0007 | `order_sagas.idempotency_key` + unique `(buyer_id, idempotency_key)`, `orders.paid_at`, `orders_status_check` (`NOT VALID`; validate with `ALTER TABLE orders VALIDATE CONSTRAINT orders_status_check` after `SELECT count(*) FROM orders WHERE status NOT BETWEEN 1 AND 5` returns 0) |

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
- A failed `PaymentSettled` leaves the order PENDING; nothing cancels it automatically.
- `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` and `ENV` are inert: only the otelgrpc stats handler is attached, and no tracer provider or exporter is configured in this repo.
- `FEATURES.yaml` references `order.v1.ListOrders`, which does not exist (the RPCs are `ListBuyerOrders` and `ListSellerOrders`).

## Runbook: commit re-sync (after deploying port-order-inventory-correctness)

Orders placed before team-order started calling `CommitReservation` have `active` reservations in team-domain, which its TTL sweep would restore. Right after deploying team-domain and then team-order, run once (same env as the server):

```bash
go run ./cmd/resync-commits -dry-run   # counts local COMMITTED reservations
go run ./cmd/resync-commits            # commits them in team-domain
```

It reads `order_reservations` (no local write) and calls `CommitReservation` per row as `service-team-order`; it is idempotent, so re-running changes nothing. Exit 0 = all committed; 2 = some rows need attention: `FAILED_PRECONDITION` (already swept in team-domain: that order's stock was given back before the upgrade) and `NOT_FOUND` are logged and skipped; other errors are safe to retry. Exit 1 = fatal (including team-domain answering `UNIMPLEMENTED`: deploy it first).

## Links

- Rules and recipe: root `AGENTS.md`.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 / 0006 / 0010 auth and zero trust, 0007 durable saga and compensation, 0008 inventory reservation model, 0009 payment-order event integration, 0013 seller demand forecasting and order warehouse integration (order events).
