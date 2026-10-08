## Why

Verified by reading `team-order`, three independent holes let an order end up in a state the business never
intended:

1. **Order status is unguarded.** `UpdateOrderStatus` only checks that the id, target status and ownership are
   present, then runs `UPDATE orders SET status=$2 WHERE id=$1` with no expected-status predicate. A buyer can
   move a `Completed` order back to `Pending` or a `Paid` order to `Cancelled` without any stock release.
   `ErrInvalidStatus` exists but only `CancelOrder` uses it, and `CancelOrder` is itself read-then-write, so two
   concurrent cancels both pass the check and both release stock. The `PaymentSettled` consumer has the same
   shape (read `Pending`, then write `Paid`), so a late `PAYMENT_SUCCESS` racing a cancel can resurrect a
   cancelled order. The shipment path discards the error of its `Shipped` update. Nothing in the database
   constrains `orders.status`.
2. **Checkout is not idempotent.** `CreateOrderRequest` carries no client key. If `CreateOrder` succeeds but
   `RemoveItems` fails (only logged) or the client retries before the cart is cleared, a second order is
   created for the same items, and because the reservation id is reused `ReserveStockIdempotent` reports
   success without decrementing stock again: the buyer is charged twice for stock held once.
3. **Silent in-memory fallback.** When the database pool is nil, `cmd/server/main.go` wires `NewInMemory*`
   repositories, and the saga repository, the payment consumer and the reservation sweeper are skipped. A
   mis-set `DATABASE_ENABLED` in staging or production therefore boots a service that looks healthy and loses
   every order on restart.

Fixing these now matters because every checkout and every order action goes through them.

## What Changes

- **team-order** (all three items):
  - Order lifecycle state machine: one allowed-transition table in the service layer. Every status write
    (`UpdateOrderStatus`, `CancelOrder`, the `PaymentSettled` consumer, the shipment flow) goes through an
    atomic, compare-and-set repository write (`UPDATE ... WHERE id=$1 AND status = ANY($allowedFrom)`); zero
    rows means a typed invalid-transition error (mapped to `FAILED_PRECONDITION`), never a silent overwrite.
  - Role rule: a buyer may only cancel (from `Pending` or `Paid`); all other transitions require the seller or
    a system caller, except `Pending -> Paid`, which is written only by a system caller (a settled payment),
    never by the seller or the buyer.
  - `CancelOrder` claims the `Cancelled` transition first (so exactly one concurrent cancel wins), then
    releases the order's reservations; a stale or out-of-order `PaymentSettled` for a non-`Pending` order is
    ignored and logged, never applied.
  - The shipment flow no longer discards the status-update error.
  - New migration: `CHECK (status BETWEEN 1 AND 5)` on `orders` (`NOT VALID`, then validated).
  - Checkout idempotency: an `Idempotency-Key` per checkout attempt, stored on `order_sagas` with a unique
    `(buyer_id, idempotency_key)`; a repeat returns the orders already created, reservation ids are derived from
    the key, and a cart-clear failure can no longer produce a second order.
  - Fail fast at startup when `ENV` is `staging` or `production` and the database is disabled; in-memory
    repositories remain for `local` and `test` only.
- **team-gateway**: forward the `Idempotency-Key` HTTP header as gRPC metadata on `CreateOrder`. Forward only;
  the gateway never stores or compares keys (Rule 2).
- **team-frontend**: `CheckoutView` generates one key per checkout attempt and sends it; retry of the same
  attempt reuses it.
- **platform-core**: no proto change (the key travels as gRPC metadata, see design D3). ADR-0007 gets a short
  addendum for the order status machine and checkout idempotency.
- **BREAKING (behavioural, internal-facing)**: transitions that were silently accepted (for example
  `Completed -> Pending`, buyer-driven `Paid -> Shipped`) are now rejected with `FAILED_PRECONDITION`;
  `CancelOrder` of a non-cancellable order returns `FAILED_PRECONDITION` instead of `INTERNAL`.

## Capabilities

### New Capabilities
- `order-lifecycle-guards`: the order status state machine and its atomic enforcement, checkout idempotency,
  and the boot-time guarantee that staging and production never run on in-memory order storage.

### Modified Capabilities
<!-- No existing order capability in openspec/specs/; nothing to modify. -->

## Impact

- Repos: `team-order` (bulk), `team-gateway` (one forwarded header), `team-frontend` (key generation),
  `platform-core` (ADR addendum only), `platform-e2e` + owning `FEATURES.yaml` files (scenarios).
- Contract: no `.proto` change; `buf lint` / `buf breaking` unaffected. The `Idempotency-Key` metadata
  contract is documented in the ADR addendum.
- Data: two `team-order` migrations: `orders.status` CHECK (`NOT VALID`, validate after a data query) and
  `order_sagas.idempotency_key` + partial unique index.
- Runtime: new `ENV` validation at boot in `team-order`; deployments setting `ENV=staging|production` with
  `DATABASE_ENABLED=false` will now refuse to start (intended).
- Architecture rules: Rule 1 (frontend only talks to the gateway), Rule 2 (gateway forwards the key, holds
  no idempotency logic), Rule 3 (all state in team-order's own DB), Rule 4 (no contract fork).
- Depends on, but does not include, `inventory-commit-and-idempotent-release` for idempotent stock release
  (see design D2 and Risks).

## Non-goals

- No stock commit/release work (separate change `inventory-commit-and-idempotent-release`); cancel here only
  orders and claims the transition and calls the release path that change makes idempotent.
- No `order.events` topic / outbox (ADR-0012 is a separate change).
- No state machine for returns or shipments; only the order status is covered.
- No automatic refund when a cancelled order receives a late payment (the event is ignored and logged; refund
  is a follow-up).
- No new `.proto` fields or RPCs and no change to auth scopes.
