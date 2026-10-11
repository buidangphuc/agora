## Why

The retired `full_team_repo` had a correct order/inventory path; agora was rebuilt from an older base and lost it. Verified
in agora's current code:

- **A placed order oversells after 15 minutes.** `team-domain` has no `committed` reservation state and no commit RPC
  (`listing.proto` has only `ReserveStock`/`ReleaseStock`). Its sweeper restores the stock of every `active` reservation
  past `DefaultReservationTTL`, including the reservation of every order already placed.
- **Release is a blind add.** `ReleaseStock` ignores `reservation_id` (`stock = stock + quantity`, reservation row
  untouched), so the sweeper later restores the same stock a second time. `CancelOrder` in `team-order` releases under a
  synthetic `cancel:<item>` id that never matches the reserve row.
- **Reserving under a released id reports success** (`ReserveStockIdempotent` returns `nil` for any existing row), so an
  unkeyed checkout retry after a compensated attempt places an order on stock that was never held.
- **Multi-seller checkout half-succeeds.** `CreateOrdersFromCart` iterates a Go map and persists each seller's order in
  its own write; a later seller's failure leaves earlier orders placed and the cart intact, so a retry duplicates them.
- **Checkout is not idempotent.** The gateway already validates `Idempotency-Key` and forwards it as `idempotency-key`
  metadata, but `team-order` never reads it and the frontend never sends one.
- **Order status writes are unguarded.** `UpdateOrderStatus` is read-check-then-write, `CancelOrder` accepts a `Shipped`
  order, two concurrent cancels both release stock, `CreateShipment` discards its status error and will ship a
  cancelled order, and the `PaymentSettled` consumer reads `Pending` then writes `Paid` (a late payment can resurrect a
  cancelled order) and commits the voucher before that check.
- **The saga view is invented.** `GetSagaState` hard-codes four steps with fabricated offsets (a `Pending` order shows
  "Payment Charged: SUCCESS"); `ForceFailSaga` ignores `fail_step` and always answers `success=true`.
- The reservation TTL and sweep interval are hard-coded, so none of this can be proven end to end.

Every checkout and cancel goes through this path.

## What Changes

- **platform-core (contract, lands first, alone):** add `rpc CommitReservation(CommitReservationRequest) returns
  (CommitReservationResponse)` to `platform.listing.v1.ListingService` with `CommitReservationRequest { string
  reservation_id = 1; }` and an empty `CommitReservationResponse`; comment the reservation lifecycle
  (`active → committed | released`) and the release-by-`reservation_id` semantics on `ReserveStock`/`ReleaseStock`.
  Additive only; `buf lint` and `buf breaking` must pass. Re-vendor and regenerate in `team-domain`, `team-order` and
  `team-gateway`. ADR-0007 and ADR-0008 addenda.
- **team-domain:** migration `0010` (reservation status `committed`, `CHECK (stock >= 0)` backstops, `NOT VALID` then
  validated); `CommitReservation`; `ReleaseStock` keyed on `reservation_id` restoring the stored quantity exactly once
  (unknown / swept / repeated = successful no-op; empty id = `INVALID_ARGUMENT`); reserve under a `released` id fails;
  `ListingStockChanged` written to the existing outbox inside the reserve / release / sweep transaction when stock
  actually changed; configurable `RESERVATION_TTL` and `RESERVATION_SWEEP_INTERVAL`.
- **team-order:** three-phase all-or-nothing checkout (reserve all → commit all in `team-domain` → place every order and
  bind its reservations in one transaction); reservation ids scoped to the saga attempt; checkout idempotency on the
  `idempotency-key` metadata (unique per buyer, replay returns the same orders, in-progress → `ABORTED`, failure frees
  the key); one transition table with actor classes, applied through a compare-and-set write on every path (RPC, cancel,
  payment consumer, shipment); cancel claims `Cancelled` first, then releases the order's original reservations (failures
  parked for the sweep) and its voucher hold; the payment consumer marks `Paid` only from `Pending` and commits the voucher
  only for an order that reached `Paid`; `orders.paid_at`; `orders_status_check`; `GetSagaState` derived from persisted
  state; `ForceFailSaga` validates `fail_step` and reports a parked release; configurable `RESERVATION_TTL` /
  `RESERVATION_SWEEP_INTERVAL`; `CommitReservation` sent as the `service-team-order` principal with `listing.write`.
- **team-frontend:** the checkout form generates one `Idempotency-Key` per checkout attempt and sends it on
  `CreateOrder`; `ABORTED` is shown as "checkout in progress, retry".
- **team-gateway:** re-vendor/regenerate only; `ListingService/CommitReservation` is answered `unimplemented` (501) by the
  embedded unimplemented handler, pinned by a test. No routing or business logic added (Rule 2).
- **compose / gitops:** `.env.example` and README entries for the new keys; a `platform-e2e` compose overlay with short
  TTLs and sweep intervals for both services. Deployed environments keep the defaults (no gitops value change).
- **BREAKING (internal / behavioural):** `ReleaseStock` without `reservation_id` is `INVALID_ARGUMENT`; illegal status
  transitions that were silently applied (cancel of a `Shipped` order, ship of a `Cancelled` one) now fail with
  `FAILED_PRECONDITION`; a multi-seller checkout with one failing seller now creates no order at all.

## Capabilities

### New Capabilities

- `inventory-reservations`: the stock reservation lifecycle owned by `team-domain` (reserve, commit, idempotent release,
  TTL sweep, configuration, stock-change events) and the `team-order` obligation to commit before placing.
- `order-checkout-correctness`: atomic all-or-nothing placement, attempt-scoped reservations, checkout idempotency on the
  client key, and the frontend's per-attempt key.
- `order-lifecycle-guards`: the order status transition table and actor classes, compare-and-set enforcement, cancel
  semantics (stock and voucher), stale payment events, and shipment creation.
- `order-read-access`: a saga view built from persisted state, and an honest `ForceFailSaga`.

### Modified Capabilities

- `edge-route-policy`: `ListingService/CommitReservation` joins the internal-only RPCs answered 501 at the edge.
- `order-upstream-principals`: `CommitReservation` is also called as the `service-team-order` principal with exactly
  `listing.write`.

## Impact

- Repos: `platform-core` (proto + ADRs), `team-domain`, `team-order`, `team-frontend`, `team-gateway` (regen + test),
  `platform-e2e` and the `FEATURES.yaml` of `team-domain`, `team-order`, `team-frontend`, `team-gateway`.
- Contract: one additive RPC and two messages in `platform/listing/v1/listing.proto`. Other vendored copies
  (`team-ai`, `team-search`, …) may stay on the previous copy; they do not call `ListingService` stock RPCs.
- Data: `team-domain` migration `0010`; `team-order` migration `0007` (`order_sagas.idempotency_key` + partial unique
  index, `orders.paid_at`, `orders_status_check NOT VALID`). Both additive.
- Events: `ListingStockChanged` starts flowing on `listing.events` (existing type, existing topic, existing outbox).
  `team-search` still ignores it until `search-stock-events` lands; nothing breaks meanwhile.
- Rules: Rule 3 (team-order reaches stock only through gRPC; the atomic placement stays inside `order_db`), Rule 4 (the
  only contract change is in `platform-core/packages/proto`), Rule 5 (stock events via the outbox on
  `listing.events`), Rule 1/2 (frontend → gateway only; gateway forwards the key it already validates).
- Deploy order: proto → `team-domain` → `team-order` → `team-frontend` (gateway regen any time after the proto).

## Non-goals

- Payment hold-back, settlement, refunds or a refund ledger, including refunding a late payment on a cancelled order or
  un-redeeming a voucher already committed by a settled payment.
- Search: consuming `ListingStockChanged`, a stock projection, `SearchHit.stock` or an `in_stock` filter (change
  `search-stock-events`, which now depends on this change for the producer side). Any other search work.
- AI-first work (tracking SDK, serving attribution, recommendations).
- Ordered (`seq`) outbox delivery, an `inventory.write` scope (agora already gates stock RPCs with
  `RequireServiceScope("listing.write")`), `GetShipmentTracking` ownership, the in-memory-storage boot guard,
  `DB_MAX_CONNS`/Kafka config hygiene and the dedupe-lookup fix: each is a separate port item.
- Order events beyond the existing `OrderPaid`/`OrderShipped` outbox; no `order.events` contract change.
- Returns/RMA and shipment status machines; a request fingerprint on idempotency keys.
