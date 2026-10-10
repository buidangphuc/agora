## Context

See `proposal.md` (Why). This change re-specifies, against agora's code, what the carried-over changes
`inventory-commit-and-idempotent-release`, `order-domain-correctness` and `order-integrity-guards` built for the retired
repo. Current state, verified by reading agora (not the old repo):

- **Contract.** `platform/listing/v1/listing.proto`: `ListingService` has `ReserveStock`/`ReleaseStock`; both requests
  already carry `reservation_id = 4`. There is no commit RPC. `ListingStockChanged { listing_id = 1; stock = 2;
  variants = 3; }` exists and is emitted by nobody. `order.proto` needs no change (`SagaStep.status` is a free string;
  `ForceFailSagaRequest.fail_step = 2` exists).
- **team-domain.** Migrations stop at `0009`. `reservations.status` is `active|released` (no CHECK). The handler gates
  stock RPCs with `RequireServiceScope("listing.write")` (service principal only). `ReserveStockIdempotent` returns `nil`
  for any existing row (including `released`) and falls back to a ledger-less decrement for an empty id. `ReleaseStock`
  ignores `reservation_id` and adds the request quantity. The sweeper restores every `active` row past
  `DefaultReservationTTL` (const 15m) every minute (`StartReservationSweeper(svc, 0, …)`). The outbox (`0006`) is used only
  by listing create/update/delete.
- **team-order.** Migrations stop at `0006_order_outbox`. `CreateOrdersFromCart` ranges over a map of seller groups and,
  per group, reserves, holds the voucher (first group), `CreateOrder` (own tx), then marks reservations `COMMITTED` in a
  separate write whose failure is only logged. Reservation ids are `sha1(buyer|cart item|listing|variant|qty)`. No
  `idempotency-key` handling exists. `UpdateOrderStatus` checks `sellerTransitions` (`Pending→Shipped`, `Paid→Shipped`,
  `Shipped→Completed`) from a prior read, then the Postgres repo does `SELECT … FOR UPDATE` and an unconditional
  `UPDATE`, writing the `OrderPaid` outbox row when the new status is `Paid`. `CancelOrder` rejects only
  `Cancelled|Completed` (a `Shipped` order can be cancelled) and releases with a synthetic `cancel:<item>` id.
  `CreateShipment` writes the shipment (+ `OrderShipped` outbox) then `_, _ = UpdateOrderStatus(Shipped)`. The payment
  consumer commits the voucher hold, then reads `Pending`, then writes `Paid`. `GetSagaState` is fabricated;
  `ForceFailSaga` ignores `fail_step`. Sweep: every minute, `FindReleasable` = `RESERVED|RELEASE_FAILED` past
  `expires_at`; nothing settles a saga left `PENDING`. `upstream/domain.go` sends stock RPCs as `service-team-order` with
  `listing.write` via `methodScopes`.
- **team-gateway.** `ListingForwarder` embeds `UnimplementedListingServiceHandler` and does not forward stock RPCs
  (spec `edge-route-policy`). It validates `Idempotency-Key` (1–255 printable bytes) and forwards it as
  `idempotency-key` metadata on `CreateOrder` only.
- **team-frontend.** `PlaceOrderForm` → `checkoutAction` (server action) → `createOrder()` in
  `src/lib/gateway/orders.ts`; no key. An `inFlight` ref already blocks a double click within one page.
  `OrderTimeline` renders unknown statuses neutrally and hides an empty timestamp.
- **E2E.** Stock RPCs are unreachable through the gateway, so every scenario drives checkout/cancel/payment through the
  gateway and observes stock with `GetListing`. The runner can read Kafka (`KAFKA_BROKERS=redpanda:9092`) and run
  `docker` (existing boot-guard steps), and has a serial lane for destructive scenarios.

## Goals / Non-Goals

**Goals:** a placed order's stock is never restored by a sweep; every stock increment is tied to one reservation and
happens at most once; a checkout yields every order or none and is retry-safe with or without a key; no status write
can bypass the transition table or lose a race silently; the saga view tells the truth; every behaviour above is provable
black-box on the local stack.

**Non-Goals (design level):** no new service, no cross-DB transaction, no workflow engine, no change to the 15-minute
default TTL, no per-aggregate outbox ordering, no `inventory.write` scope, no change to `order.proto`.

## Decisions

### D1 — Contract: one additive RPC, landed first and alone

```proto
// Lifecycle (team-domain owns it): active → committed | released; committed → released.
// Only `active` reservations expire. Stock is restored once, when a reservation becomes released.
rpc CommitReservation(CommitReservationRequest) returns (CommitReservationResponse);

message CommitReservationRequest { string reservation_id = 1; }
message CommitReservationResponse {}
```

Names checked against agora's protos: no `CommitReservation*` exists in package `platform.listing.v1` (the promotion
service has its own in `platform.promotion.v1`, no clash). Comments on `ReserveStockRequest.reservation_id` (required;
reserving a released id fails) and `ReleaseStockRequest` (keyed by `reservation_id`; `listing_id`, `variant_id`,
`quantity` are ignored and kept for wire compatibility). No field is renumbered or removed, so `buf breaking` (FILE)
passes. The PR lands in `platform-core` before any consumer change; `team-domain`, `team-order` and `team-gateway`
re-vendor `proto/` and run `buf generate` in their own tree (ADR-0001). Other repos' copies stay as they are (they do not
use stock RPCs; several are already behind).
*Alternative:* tell `team-domain` of the commit through an event — rejected: the sweep could fire before the event lands.
Raising the TTL only delays the oversell.

### D2 — Release by reservation, one statement, same predicate as the sweep

`UPDATE reservations SET status='released', released_at=now() WHERE reservation_id=$1 AND status IN ('active','committed')
RETURNING listing_id, variant_id, quantity`, then restore that quantity in the same transaction. Zero rows → successful
no-op (warn log). The sweep keeps `status='active' AND expires_at <= now` with `FOR UPDATE SKIP LOCKED`, so sweep and
release can never both restore. `committed` stays releasable because cancel-after-payment must return stock. Commit is
`UPDATE … SET status='committed' WHERE reservation_id=$1 AND status='active'`, then a lookup to tell
committed/released/unknown apart. Migration `0010` adds `CHECK (status IN ('active','committed','released'))` and
`CHECK (stock >= 0)` on `listings`/`listing_variants`, `NOT VALID`, validated in a later step after a count query.
*Alternative:* a dedupe table keyed `(reservation_id, action)` — rejected, the reservation row already is the ledger.

### D3 — Reserve: released id fails, empty id is invalid

`ReserveStockIdempotent`: existing `active|committed` → success no-op; existing `released` → `FAILED_PRECONDITION`
(the caller must use a new id); empty id → `INVALID_ARGUMENT` in the handler, and the ledger-less fallback is deleted.
`ReleaseStock` with an empty id → `INVALID_ARGUMENT`. `team-order` always sends ids, so this ships together with D7.

### D4 — `ListingStockChanged` is produced here; consumption stays in `search-stock-events`

The producer side is part of the transactions this change rewrites (reserve, release, sweep), and `listing-write` already
requires stock changes to be recorded in the same transaction as their event, so emitting belongs here. The
service-layer `enqueue` runs inside each stock transaction, only when a row actually changed stock, one event per
listing per transaction, payload = stock after the change (plus the changed variant), envelope on `listing.events` keyed
by `listing_id`, same relayer. Events carry absolute values, so a consumer that guards on `occurred_at` (as
`search-stock-events` D2 does) tolerates the existing at-least-once, not-strictly-ordered relay. `team-search` keeps
ignoring the type until `search-stock-events` lands; that change's "domain now emits" premise becomes true with this one.

### D5 — Same two config keys in both services; never fail boot on them

`RESERVATION_TTL` (default `15m`) and `RESERVATION_SWEEP_INTERVAL` (default `1m`) as Go durations in each service's
`internal/config`, `.env.example` (drift tests already exist in both repos), README, and an INFO line at sweeper start
with the effective values. Invalid or non-positive → default plus WARN, in every `ENV`: a typo in a TTL should not take a
production service down, and the default is the safe value. `team-domain` passes the TTL to `ReserveStockIdempotent` and
the interval to `StartReservationSweeper`; `team-order` wires `WithReservationTTL` and `runReservationSweeper`.

### D6 — Three-phase, all-or-nothing checkout; placement and binding in one transaction

`CreateOrdersFromCart`: group by seller, **sort seller ids**, pre-generate one order id per group, then

1. **Phase A — reserve all.** Per item: persist the reservation (`PENDING`) → `ReserveStock` → `RESERVED`. Voucher hold
   on the first sorted group, as today. A declined reserve → `ErrInsufficientStock` (`RESOURCE_EXHAUSTED`).
2. **Phase B — commit all** in `team-domain` (`CommitReservation`). `FAILED_PRECONDITION` →
   `ErrReservationLost` (`FAILED_PRECONDITION`, "item no longer reserved"); `UNIMPLEMENTED` is a failure too (no legacy
   mode: deploy order guarantees the RPC exists).
3. **Phase C — place.** New port `OrderPlacer.PlaceOrders`: one `pgx` transaction inserts every order and its items,
   binds reservations `RESERVED → COMMITTED` with their `order_id` requiring `RowsAffected == len(ids)` (else roll back
   with `ErrReservationLost`: the sweep released one meanwhile), and marks the saga `COMPLETED`. In-memory: same contract
   under one mutex.

Any failure in A/B, or a definite failure in C, runs `failAndCompensate` (release every `RESERVED|COMMITTED`-in-domain
reservation of the saga by id — committed ones are releasable — release the voucher hold, saga `COMPENSATED`, key
cleared). An ambiguous C error is reconciled on a fresh context by looking up the pre-generated order ids (all → placed,
none → compensate, partial/lookup error → `INTERNAL`, no release). The cart is cleared after C (best effort, retried by
a keyed replay).
*Dropped from the old design:* the "intended order id" sweep repair. It existed to heal an order whose binding write
failed separately; with orders, bindings and saga status in one transaction that state cannot occur, and a crash before
C leaves `RESERVED` rows with no order, which the sweep correctly releases.
*Alternatives:* per-seller partial results (needs a proto change and a partial saga state; rejected); cancel earlier
orders on a later failure (leaves phantom cancelled orders and side effects; rejected).

### D7 — Reservation ids are scoped to the saga attempt, never to the key

`ReservationID = sha1(ns, saga_id | cart_item_id | listing | variant | qty)`, keyed or not. Two attempts never share a
reservation, so D3's "released id fails" never blocks a legitimate retry. The key is not needed in the id: a keyed
replay never reserves (D8), and concurrent same-key requests collapse on the saga's unique index before reserving.

### D8 — Idempotency key stored on the saga, unique per buyer

Migration `0007`: `order_sagas.idempotency_key TEXT NULL` + `CREATE UNIQUE INDEX … (buyer_id, idempotency_key) WHERE
idempotency_key IS NOT NULL`. The handler reads `idempotency-key` metadata, trims and re-validates it (1–255 printable
ASCII, same rule as the gateway; defence in depth). `CreateSaga` is insert-or-lookup (`ON CONFLICT DO NOTHING RETURNING`,
then select). Existing saga: `COMPLETED` → return its orders (`order_reservations.order_id` by `saga_id`) and re-attempt
the cart clear; `PENDING` → `ABORTED`; `COMPENSATED|FAILED` never matches because compensation sets the key to `NULL`. The
`team-order` sweep gains a stale-saga pass: a saga `PENDING` past the reservation TTL with no orders is compensated (its
reservations released, key cleared), so a crash never pins a key. Same key with a different cart returns the original
orders (standard idempotency semantics; the frontend changes the key when inputs change).
*Alternative:* a separate `idempotency_keys` table with a stored response — more general, needs TTL cleanup, stores a
response that can go stale; rejected.

### D9 — Transition table with actor classes, compare-and-set write

One table in `internal/service` (see spec `order-lifecycle-guards`). Decisions specific to agora:
- **`Pending → Shipped` (seller) is kept.** Agora's COD flow ships unpaid orders; the old table dropped it, which would
  break COD fulfilment.
- **Cancel only through `CancelOrder` (buyer) / `ForceFailSaga` (buyer or admin)**, as agora does today; neither the
  seller nor `UpdateOrderStatus` may cancel. The old table let a seller cancel; adopting that would be a product change
  nobody asked for.
- **Admin acts as seller** on `UpdateOrderStatus` (today's `isAdminOrUser`).
- Error order: stranger → `PERMISSION_DENIED`; target not permitted to the actor class at all → `PERMISSION_DENIED`;
  permitted target from the wrong status → `FAILED_PRECONDITION`.

Repository: `UpdateOrderStatusFrom(ctx, id, to, allowedFrom []OrderStatus, tracking, paidAt)` =
`UPDATE orders SET status=$2 … WHERE id=$1 AND status = ANY($3) RETURNING …`, inside the existing transaction that writes
the `OrderPaid` outbox row for `Paid`; zero rows → `SELECT` to return `ErrOrderNotFound` vs `ErrStatusConflict`. The
unconditional `UpdateOrderStatus` is removed from the interface so no caller can bypass the guard. Migration `0007` adds
`orders_status_check CHECK (status BETWEEN 1 AND 5) NOT VALID` (validated later) and `orders.paid_at TIMESTAMPTZ NULL`.

### D10 — Cancel: claim, then release by original ids, then voucher

`CancelOrder`: `UpdateOrderStatusFrom(Cancelled, [Pending, Paid])`; only the winner loads the order's reservations
(`order_reservations` by `order_id`, `COMMITTED|RELEASE_FAILED`) and releases each by id on a fresh context with the
existing retry; a failure parks the row `RELEASE_FAILED`; then `releaseVoucher(order.ID)` if the order has a voucher
(log only). The sweep adds "reservations of `Cancelled` orders still `COMMITTED`" to `FindReleasable`, bounding the
claim→release crash window. `CancelOrder` returns whether a release is parked (`ReleasePending`) for D13.
*Alternative:* release then flip — the order can ship in between, and two cancels both release; rejected.

### D11 — Payment consumer: CAS first, voucher only for a paid order

`apply`: `UpdateOrderStatusFrom(Paid, [Pending], paidAt=now)`. Conflict → acknowledge, log `order_id` + current status.
Then, if the order has a voucher and `paid_at` is set (this write or an earlier delivery), `CommitReservation` on the
promotion hold (idempotent); a commit error returns for redelivery, and the redelivery's CAS conflicts but `paid_at` is
set, so the commit is retried. This closes the old design's documented residual (voucher committed before the status
check) and makes the voucher scenario provable: a cancelled order's hold is released (D10) and never committed.

### D12 — Shipment: claim `Shipped` first

`CreateShipment`: `UpdateOrderStatusFrom(Shipped, [Pending, Paid], tracking)`; conflict → `FAILED_PRECONDITION`, no
shipment; then insert the shipment and its `OrderShipped` outbox row. A shipment insert failure after a won claim returns
`INTERNAL` and is logged; a retried `CreateShipment` then conflicts. *Trade-off:* shipment and order rows are in the same
DB but different repositories; a shared transaction would need a placer-style port — not worth it for a seller action
that can be inspected and fixed by hand.

### D13 — Saga view from persisted facts; honest force-fail

`GetSagaState` per the table in spec `order-read-access` (order row, its reservations, `paid_at`); no invented offsets,
`compensation_reason` "order cancelled". `ForceFailSaga`: validate `fail_step ∈ {"", "payment", "shipping"}` before any
write, cancel via D10, `success = !ReleasePending`, message names the parked release. Existing step names stay; the new
`SKIPPED` renders neutral in `OrderTimeline` (no frontend change needed).

### D14 — Frontend: one key per attempt, carried through the server action

`PlaceOrderForm` holds a key (`crypto.randomUUID()`) in a ref, created on mount and renewed after a success or when the
submitted address / payment method / voucher / items differ from the previous submit. It passes the key to
`checkoutAction(…, idempotencyKey)` → `createOrder(…, idempotencyKey)`, which sends `Idempotency-Key` as a call header on
the Connect client. `aborted` maps to a "checkout in progress, try again" message.

### D15 — Gateway: regenerate, pin, nothing else

After re-vendoring, `ListingService/CommitReservation` is served by the embedded `UnimplementedListingServiceHandler`
(501, no upstream call). A case is added to `unrouted_test.go`. No forwarder, no policy entry.

### D16 — E2E approach

All scenarios go through the gateway (Connect JSON) or the UI, never a service port.
- **Short TTL overlay**: `platform-e2e/compose/order-inventory.override.yaml` sets `RESERVATION_TTL=20s`,
  `RESERVATION_SWEEP_INTERVAL=2s` on `team-domain` and `team-order`, used as
  `docker compose -f docker-compose.yaml -f platform-e2e/compose/order-inventory.override.yaml up -d`. It is safe for the
  whole suite because placed orders are committed and immune to the sweep; TTL scenarios wait TTL + 2 × interval + margin.
- **Parked release**: serial lane, `docker stop team-domain-svc` → act → `docker start` → poll stock. Stopping (not
  pausing) makes calls fail fast with `UNAVAILABLE` instead of hanging until the release timeout.
- **Stock events**: consume `listing.events` from the current offset, filter by envelope type and key.
- **Replay**: Playwright captures the checkout server-action request and re-sends it unchanged.
- **Concurrency**: two requests started together with a barrier; assertions accept either legal winner.
- Fault cases the edge cannot reach (ambiguous commit in Phase C, crash between claim and release, commit refused after
  a domain sweep, outbox rollback) are covered by repository/service tests against Postgres
  (`TEST_DATABASE_URL`), not by e2e, and the specs state them only as requirement text.

## Risks / Trade-offs

- [Mixed versions: a new `team-domain` with an old `team-order` — old cancels release under synthetic ids, now an
  unknown-id no-op, so their stock is not returned until reconciled] → deploy `team-domain` and `team-order` back to back;
  under-restore is the safe direction (never oversells). An old `team-order` still never commits, so placed orders keep
  being swept until it is upgraded, exactly as today.
- [Orders placed before the upgrade have `active` domain reservations] → a one-off `team-order` command (`cmd/resync-commits`)
  calls `CommitReservation` for every local `COMMITTED` reservation; idempotent; run right after deploy. Orders older
  than the TTL were already swept (today's bug); the command logs `FAILED_PRECONDITION` for them and they are left as is.
- [Holding every seller's stock until Phase C] → same TTL as today; one cart has few sellers; no network call inside the
  Phase C transaction.
- [A shop voucher for a seller that is not first in sorted order is still rejected] → unchanged behaviour (today the
  first group is random); out of scope.
- [ProcessMockPayment may refuse a payment for a cancelled order, making the late-payment scenarios unreachable] → the
  e2e creates the payment before cancelling; if `team-payment` still refuses, the scenario is `xfail(strict=True)` naming
  the payment-side check and the consumer behaviour stays covered by consumer tests.
- [Short TTL overlay changes timing for unrelated suites] → only uncommitted holds are affected; run the full suite twice
  with the overlay before archive (§5 stability).
- [Shipment insert can fail after the `Shipped` claim] → logged, `INTERNAL`, manual fix (D12).
- [`ListingStockChanged` events start flowing before anyone consumes them] → ignored by `team-search` (`default` branch);
  topic volume grows by one event per stock change.

## Migration Plan

1. `platform-core`: proto PR (`buf lint`, `buf breaking --against main`), ADR-0007/0008 addenda. Merge first, alone.
2. Re-vendor + regenerate in `team-domain`, `team-order`, `team-gateway` (generated code never hand-edited).
3. `team-domain`: migration `0010` (`NOT VALID`), code, deploy; then `VALIDATE CONSTRAINT` after a zero-violation count.
4. `team-order`: migration `0007`, code, deploy immediately after step 3; run `cmd/resync-commits` once; validate
   `orders_status_check` after a count query.
5. `team-gateway`: regenerated build (any time after step 1). `team-frontend`: key generation (after step 4; harmless
   earlier because a missing key keeps today's behaviour).
6. Rollback: `team-frontend`/`team-gateway` independently. `team-order` alone can roll back (it stops committing; today's
   sweep oversell returns). Rolling back `team-domain` requires rolling back `team-order` first (it calls
   `CommitReservation`). Migrations are additive; constraints can be dropped.
