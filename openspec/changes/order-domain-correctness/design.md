## Context

See `proposal.md` for motivation and `specs/` for requirements. Item numbers (#1..#11) are the audit numbers.
Everything below was re-read in the code at the tip of the named branches; claims that turned out false or
incomplete are listed under "Corrections to the audit". Nothing was executed against a live stack.

Verified current state:

- **Placement** (`team-order/internal/service/order.go`): sellers are iterated with `range` over a Go `map` (random
  order). Per seller group: persist reservation (`PENDING`) -> `ReserveStock` -> `RESERVED` -> voucher hold (first group
  only) -> `CommitReservation` in team-domain -> `orderRepo.CreateOrder` (own tx) -> `sagaRepo.CommitReservation` (a
  **separate** write, failure only logged, ~L372-378) . `failAndCompensate` marks the saga `COMPENSATED`, which clears
  the idempotency key (`UpdateSagaStatus`), so a same-key retry starts a new saga with new reservation ids.
- **Sweep** (`saga.go`, `repository/saga.go`): `FindReleasable` returns `RESERVED|RELEASE_FAILED` past `expires_at` with no
  look at orders; `compensate` releases them by `reservation_id`. team-domain releases `active` **and** `committed`
  reservations (ADR-0008), so a release of a placed order's reservation restores its stock.
- **Reserve** (`team-domain` handler): out-of-stock is `ReserveStockResponse{success:false}` with a **nil** error; a
  released id is `FAILED_PRECONDITION`; an out-of-stock reserve rolls its ledger row back with the tx. team-order does
  `_, rerr := ReserveStock(...)`, ignores `success`, and wraps any `rerr` as `ErrInsufficientStock` -> `ResourceExhausted`.
  The unit fake returns `success:false` **and** an error, so it never exercised the real contract.
- **Stock RPC exposure**: `team-gateway` (`internal/edge/listing.go`) forwards `ReserveStock`/`ReleaseStock`; its auth
  interceptor never rejects (anonymous -> `PUBLIC_SCOPES` = `listing.read,search:read`); `team-domain`'s three stock
  handlers do no `RequireScopes`; `ReserveStockIdempotent("")` falls back to `ReserveStock` (plain decrement, no ledger
  row). team-order's upstream interceptor forwards the **incoming** (buyer) principal when one exists and otherwise
  injects `service-team-order` with `listing.read,listing.write,identity.read,identity.write`.
- **Voucher** (`team-promotion`): `ValidateAndReserve` creates a hold and does **not** touch quota; quota is consumed
  (`voucher.used++`) only by `CommitReservation`, which team-order calls from the `PaymentSettled` consumer;
  `ReleaseReservation` of a **committed** hold returns `released=false` (never releases).
- **Order reads** (`handler/order.go`): `GetOrder` checks ownership only `if principal, ok := ...; ok` and never allows
  admin. `GetShipmentTracking` has no principal or ownership check at all. The frontend calls both from the authenticated
  order-detail page. `GetSagaState` hard-codes four steps and offsets (`CreatedAt+50ms`, ...); every non-cancelled order,
  `Pending` included, gets "Payment Charged: SUCCESS". `ForceFailSaga` ignores `fail_step` and always answers
  `success:true`, although `CancelOrder` swallows a parked release. The frontend `OrderTimeline` renders unknown step
  status strings neutrally (grey), so a new status string needs no frontend change.
- **Config** (`team-order/internal/config`): tag `DB_MAX_CONns` (case-sensitive `os.LookupEnv`) makes `DB_MAX_CONNS`
  a no-op while README and `.env.example` already say `DB_MAX_CONNS`; the five Kafka keys are read ad hoc in
  `bootstrap/kafka.go` and commented out in `.env.example`; there is no `CheckEnvExample`-style drift test in
  team-order (only a per-key test). `PostgresProcessedEventRepository.IsProcessed` returns `false, nil` on any scan error.
- **Outbox** (`team-domain/internal/repository/outbox_pg.go`, `team-payment/internal/repository/outbox_pg.go`): the claim is
  `UPDATE ... WHERE event_id IN (SELECT ... ORDER BY available_at, created_at FOR UPDATE SKIP LOCKED LIMIT $1) RETURNING ...`;
  `RETURNING` order is unspecified. `created_at`/`available_at` default to `now()` = **transaction start**, not commit or
  lock time. The relayer produces the claimed slice in order and, on failure, only schedules backoff for that one row.
- **Scopes**: services enforce `listing.read|write`, `search:read|write`, `engagement:read|write` (all granted by
  `team-identity/internal/authz/scopes.go`) and `recommendations:read` (`team-ai` `recommend.py`), which is granted to
  nobody. Tokens carry the scopes chosen at login (TTL 3600s, no refresh flow).

### Corrections to the audit

- **#3 (voucher)**: the claim "a cancelled Pending/Paid order leaks voucher quota" is only half right. A cancelled
  **Pending** order leaves a dangling `reserved` hold that consumes **no** quota (quota counts `used`, bumped at commit).
  A cancelled **Paid** order's hold was committed and team-promotion refuses to release it, so its quota *is* consumed and
  cannot be returned without a team-promotion change. This change releases the hold on cancel (hygiene, matches ROADMAP and
  migration 0005's stated intent) and records the committed-redemption refund as an open question.
- **#7 (stock RPC scope)** is stronger than ADR-0010 implies: the exposure is not only "a pod that reaches the port"; the
  **public gateway routes the RPCs** and the domain trusts whatever principal arrives, so anonymous internet callers can
  reach an unledgered decrement. The domain-side gate closes it regardless of gateway routing.
- **New finding (#2b)**: `success=false` is ignored by team-order (see Context); fixed together with #2.
- **#10 is broader than the claim query**: even with a stable `RETURNING` order, `now()` (transaction start) can invert
  two events written by overlapping transactions, and a backed-off event is overtaken by its aggregate's later events.
  The guarantee is therefore specified per aggregate and implemented with an insert-order sequence.
- **#9**: also note `IsProcessed`/`MarkProcessed` key the ledger on `event_id` alone (the `consumer` argument is unused);
  harmless today (one consumer), recorded, not changed.

## Goals / Non-Goals

**Goals:**

- A placed order's stock is never released by the sweep, under any failure of the placement write, the commit
  acknowledgement, or a process crash.
- A checkout either produces every order it was asked for or none, and is retry-safe with and without a key.
- Stock can be changed only by the order saga; every decrement has a ledger row.
- Outbox consumers see a listing's events in write order.
- Read surfaces describe real state and enforce ownership.

**Non-Goals (design-level):**

- No new service, no workflow engine, no cross-DB transaction, no proto shape change.
- No change to the 15-minute TTL values or the compensation retry/backoff policy.
- No global (cross-aggregate) ordering, no exactly-once delivery (still at-least-once).
- No per-seller partial-success response (rejected in D2).

## Decisions

### D1 — Place orders and bind reservations in one transaction; reservations know their order from the start (#1)

Root cause: the order row and the reservation's "belongs to order X" fact are two writes; the second can fail after the
first is durable. Fix in two layers:

1. **Prevent.** New repository port `OrderPlacer.PlaceOrders(ctx, []PlacedOrder{Order, ReservationIDs})` in team-order.
   Postgres implementation: one `pgx` tx that inserts every order and its items (existing insert logic refactored to take
   a `pgx.Tx`), then `UPDATE order_reservations SET status = COMMITTED, order_id = $o WHERE id = ANY($ids) AND status =
   RESERVED` per order and requires `RowsAffected == len(ids)`; any shortfall (a reservation already released by a
   concurrent sweep) rolls everything back with `ErrReservationLost`. In-memory implementation: same contract under one mutex
   over the shared stores. `CreateOrder`'s other callers keep using `OrderRepository.CreateOrder`.
2. **Heal.** `CreateReservation` now stores the **intended** `order_id` (the pre-generated order id of its seller group)
   while the status is still `PENDING/RESERVED`. `compensate` (used by checkout compensation, the TTL sweep and the stale-saga
   settle) first looks the intended order up: order exists and is not `Cancelled` -> **repair** (bind the reservation as
   `COMMITTED`, log WARN, count `order_reservation_repaired_total`) and never call `ReleaseStock`; order missing or
   `Cancelled` -> release as today; lookup error -> skip this tick (never release on doubt).

This needs no migration: `order_reservations.order_id` exists, is nullable and is only read in `ListReservationsByOrder`,
`FindHeldByCancelledOrders` and `replayCheckout`, which all filter on `COMMITTED`/`RELEASE_FAILED` statuses, so an intended id on a
`RESERVED` row is inert for them. Legacy rows created before this change have no intended id; they cannot be healed by the
sweep, so task 7.5 adds a one-off drift query (`orders` with no bound reservation) and a runbook step.

*Alternatives:* (a) **retry + park** the local commit (like release): rejected as the only fix - it narrows but does not
close the window (crash between the order insert and the retry still oversells) and needs a new "parked commit" state;
(b) **sweeper skips reservations of buyers who have a recent order**: heuristic, wrong under multi-item carts; (c) **stop the
domain releasing `committed` reservations**: breaks cancel-after-payment (ADR-0008) and moves the fix to the wrong service;
(d) **two-phase with a `placing` order status**: extra status visible to every reader for no gain over a single tx.

### D2 — Multi-seller checkout is all-or-nothing in three phases (#4)

New flow in `CreateOrdersFromCart` (idempotency, cart load, self-purchase check unchanged):

1. **Group and order.** Group by seller, **sort seller ids**, pre-generate one order id per group.
2. **Phase A - reserve everything.** For each group, each item: persist reservation (`PENDING`, intended order id), `ReserveStock`,
   classify the outcome (D5), mark `RESERVED`. Place the voucher hold once, for the first sorted group, exactly as today.
   Any failure -> `failAndCompensate` (releases every reservation of the saga).
3. **Phase B - confirm everything.** `CommitReservation` in team-domain for every reservation of every group (fails
   closed as today; `UNIMPLEMENTED` legacy tolerance kept). Any failure -> compensate all (committed ones are
   releasable).
4. **Phase C - place.** `PlaceOrders` for all groups in one tx (D1). Success -> saga `COMPLETED`, clear the cart, return
   the orders. Failure -> D3.

So no order exists before every group is held and confirmed, and if a late failure happens no order exists either
(one tx). The earlier "release only the failing seller's un-committed reservations" scoping of ADR-0007 (M7) is no longer
needed because nothing is persisted per seller.

*Alternatives:* (a) **per-seller partial results** (today's behaviour, made explicit): the wire response carries only
`orders`, so a partial result needs a proto change or a silent drop, a saga `PARTIAL` state, key semantics for "replay
the failed sellers", and a cart that is partly cleared - rejected as complexity with no buyer benefit; (b) **cancel the
earlier orders when a later seller fails**: leaves `Cancelled` orders the buyer never asked for in their history, sends
notification/payment side effects for orders that "existed", and still has a window; (c) keep sequential commit but
clear the idempotency key only when no order exists: does not stop the partial orders themselves.

Trade-off: stock for all sellers is held together for the (short) duration of the checkout, and a single failing seller
fails the whole cart. Both already describe the common case today (any failure after the first seller errors the call).

### D3 — Ambiguous placement outcome is reconciled before compensating (#1)

`PlaceOrders` can return an error although the tx committed (lost commit ack, context deadline). Compensating then
would release the stock of placed orders. On a `PlaceOrders` error the service, on a fresh context, looks up every
pre-generated order id: **all exist** -> treat as placed (same path as success); **none exist** -> `failAndCompensate`;
**some/lookup error** -> do not compensate, log ERROR and return `Internal`; D1's sweep repair then does the right thing
(order exists -> bind; does not -> release after TTL). Pre-generated ids make this possible without any new column.

### D4 — Unkeyed reservation ids include the saga attempt id (#2)

`ReservationIDWithKey` with an empty key becomes `sha1(ns, "attempt" | buyer | saga id | item id | listing | variant | qty)`.
Every attempt therefore has its own ids; a retry after a compensated attempt reserves normally. Legacy `ReservationID`
is removed (or kept test-only) once nothing calls it; ids already stored are unaffected.

What is lost: the crash-retry stock dedupe of unkeyed checkouts. It only ever protected **stock**, not the order: an
unkeyed retry after a crash created a **second order** on stock held once (the exact defect `order-integrity-guards`
describes). Now a duplicate attempt holds its own stock, and an abandoned attempt's hold is returned by
`settleStalePendingSagas` after the TTL. Clients that need crash-safe retry send `Idempotency-Key` (unchanged).

*Alternatives:* (a) keep deterministic ids and only map `FAILED_PRECONDITION` distinctly: the retry still can never
succeed - it converts a wrong error into a permanent dead end; (b) keep deterministic ids and retry once with a random
suffix on `FAILED_PRECONDITION`: same end state with extra control flow and a second id scheme; (c) bump a per-cart-item
generation counter in team-order's DB: new state to keep consistent with the saga for the same effect as saga id.

### D5 — Classify the reserve answer by meaning (#2b)

One function maps `(resp, err)`:

| Domain answer | team-order outcome | Reservation row | gRPC to client |
|---|---|---|---|
| `err == nil && success` | reserved | `RESERVED` | - |
| `err == nil && !success` (out of stock) | `ErrInsufficientStock` | `FAILED` | `RESOURCE_EXHAUSTED` |
| `FAILED_PRECONDITION`, `NOT_FOUND` | `ErrItemUnavailable` | `FAILED` | `FAILED_PRECONDITION` |
| `INVALID_ARGUMENT`, `PERMISSION_DENIED`, other non-transport | `Internal` (caller bug) | `FAILED` | `INTERNAL` |
| `UNAVAILABLE`, `DEADLINE_EXCEEDED`, transport errors | `ErrStockUnavailable` | **`RELEASE_FAILED` (parked)** | `UNAVAILABLE` |

A transport error leaves it unknown whether the domain applied the reserve, so the row is parked for the idempotent
release by id (a no-op when nothing was held) instead of `FAILED`, which the sweep ignores - otherwise a reserve whose
response was lost leaks stock until domain's own 15-minute sweep. The unit fake is corrected to the real contract
(`success=false` with nil error), with an error variant kept for transport failures.

### D6 — Release the voucher hold after winning the cancel claim (#3)

In `CancelOrder`, after `UpdateOrderStatusFrom(Cancelled)` succeeds and the stock release has run (same fresh context with
the release timeout), if `claimed.VoucherCode != ""` call `releaseVoucher(ctx, claimed.ID)` (the hold id is the order id).
This covers the `CancelOrder` RPC, `UpdateOrderStatus -> Cancelled` (which delegates to `CancelOrder`) and `ForceFailSaga`.
Best-effort and idempotent: a failure is logged and never fails the cancel; `released=false` (committed hold) is logged
at info as "redemption already committed; not reversed". Because a missed release leaves a hold that consumes no quota,
a durable retry/park mechanism for vouchers is not warranted.

*Alternatives:* (a) release before the claim: a lost claim would have freed a voucher of a live order; (b) park failed
voucher releases in a table and sweep: new durable state for a hold that costs nothing while dangling; (c) make
`ReleaseReservation` undo a committed redemption: a team-promotion behaviour change (open question, not done here).

Known residual (out of scope): the `PaymentSettled` consumer commits the voucher **before** its `Pending -> Paid` compare-and-set, so a
cancel racing the payment can leave a committed redemption on a cancelled order. Moving the commit after the CAS is a
consumer change deferred with the refund question.

### D7 — Saga view from persisted facts; ForceFailSaga reports the release (#5)

Chosen: **make it truthful**, not merely label it synthetic. Reasons: the order-detail page shows this timeline to buyers;
a `Pending` order showing "Payment Charged: SUCCESS" is a correctness bug, not a demo quirk; and the real data already
exists (`order_reservations`, `orders`). Derivation (no proto change; status strings stay free-form, `SKIPPED` is new and renders neutral):

| Step | Source | Status |
|---|---|---|
| Order created | `orders.created_at` | `SUCCESS` |
| Stock reserved | earliest reservation of the order | `SUCCESS` (no reservation rows: `SUCCESS` without timestamp, detail says "predates saga tracking") |
| Payment | `orders.status`, new `orders.paid_at` | `Pending` -> `PENDING`; paid-or-later -> `SUCCESS` (+`paid_at` if present); `Cancelled` with `paid_at` -> `SUCCESS`; `Cancelled` without -> `SKIPPED` |
| Confirmation (non-cancelled) | `orders.status` | `Pending` -> `PENDING`; else `SUCCESS` (no timestamp) |
| Compensation (cancelled) | reservations of the order | all `RELEASED` -> `COMPENSATED` (ts = last release), any still `COMMITTED`/`RELEASE_FAILED` -> `PENDING` ("stock release pending retry"), `is_compensated` only when all released |

Timestamps exist only where recorded; the rest are omitted. `compensation_reason` becomes a neutral "order cancelled"
(the service does not know why). Migration `0007` adds nullable `orders.paid_at`, set inside the `Pending -> Paid`
compare-and-set. `ForceFailSaga`: `fail_step` must be empty, `payment` or `shipping` (else `INVALID_ARGUMENT`; both cancel
through `CancelOrder`, the step name is echoed in the message); `CancelOrder` exposes whether any release is still parked
(`CancelResult.ReleasePending`), and the handler answers `success=false` with a "release pending retry" message in that case.

*Alternative considered:* keep the synthetic view and mark it simulated in the contract (proto comment already says so).
Rejected: cheap, but leaves a buyer-facing page lying; the truthful derivation is ~1 function plus one column. Fallback if
schedule forces it: ship only the `ForceFailSaga` honesty and the `Pending` payment-step fix.

### D8 — Order reads: authenticated, owner-or-admin, one rule (#6)

`GetOrder` and `GetShipmentTracking` call `RequirePrincipal` first (the gateway's anonymous principal is already
rejected there), fetch, then apply `isAdminOrUser(principal, buyer, seller)`. `GetShipmentTracking` resolves the order
from the shipment for every identifier kind (tracking code, order id, shipment id) and applies the same check; a tracking code
is not treated as a bearer secret (it is derived from the order id prefix and a clock value). RPC audit (all in
`platform.order.v1.OrderService`, confirmed by reading):

| RPC | Principal | Ownership rule |
|---|---|---|
| CreateOrder, Get/Add/Update/Remove/Clear cart, Reorder | required | caller's own cart/order |
| ListBuyerOrders / ListSellerOrders | required | ids taken from the principal |
| GetOrder | **was optional -> required** | buyer, seller, **admin (new)**, or a `service` principal holding `order.read` (D8a) |
| UpdateOrderStatus | required | buyer/seller actor per transition table (admin not an actor; unchanged) |
| CancelOrder | required | buyer only (unchanged) |
| CalculateShippingFee | none (pure function) | - |
| GetSagaState / ForceFailSaga | required | buyer or admin |
| CreateReturnRequest | required | buyer of the order |
| GetReturnRequest | required | buyer, seller, admin |
| UpdateReturnStatus | required | seller or admin |
| CreateShipment | required | seller or admin |
| GetShipmentTracking | **was none -> required** | buyer, seller, admin (new) |

`isAdminOrUser` accepts scopes `admin`, `order.admin`, `all`; identity issues only `admin`. The other two are dead
today and are left alone (removing them is a separate cleanup). Tracking by code without login is not a current user
journey (the only caller is the authenticated order page); see open question if a public tracking link is wanted.

### D8a — Service principal with `order.read` may read orders (regression found on the live stack)

`team-payment` reads the order it is paying (`CreatePayment`, wallet credit on settle) without any principal, which D8 now
rejects, so every payment of a real pending order failed as "order not found". Decision: `GetOrder` also accepts a principal
of type `service` holding the new service-only scope `order.read` (read-only, no other RPC is widened; granted to no
identity role, asserted by the team-identity negative test next to `inventory.write`). `team-payment` presents
`service-team-payment` / `service` / `order.read` on its order reads (same mechanism as `service-team-order` /
`inventory.write` towards `team-domain`) and keeps enforcing that the order's buyer is the caller of `CreatePayment`.
`team-payment` also stops mapping every upstream error to "order not found": `NOT_FOUND` stays `ErrOrderNotFound`, any other
code surfaces with its real gRPC code. Rollout: `team-order` first, then `team-payment`. Other callers of `GetOrder`
without a principal still get `UNAUTHENTICATED`. `team-engagement` purchase verification was in that group; it was later
moved to `service-team-engagement` / `order.read` by `service-authz-hardening` (D8), like `team-payment`.

### D9 — Stock RPC gate and service principal (#7)

- **Scope name `inventory.write`**, enforced with `RequireScopes` as the first statement of `ReserveStock`, `ReleaseStock` and
  `CommitReservation` in team-domain. `listing.write` is rejected as the gate: sellers hold it, and a seller could then drain a
  competitor's stock.
- It is held only by the order service's synthesised principal; `team-identity` must never grant it (negative test, see the `auth` delta).
- **team-order** marks stock calls with an explicit context marker (`upstream.AsService(ctx)`) that makes the existing
  client interceptor send `service-team-order` / type `service` / scopes `inventory.write` **instead of** the forwarded buyer
  principal (other metadata such as `x-request-id`/trace is still forwarded). The domain client wrapper applies the marker to
  exactly the three stock methods; `GetListing` keeps today's forwarding. Background paths (compensation, sweep) already
  have no incoming principal and get the same principal; `inventory.write` is added to the default service scope list.
- `ReserveStock` with an empty `reservation_id` -> `INVALID_ARGUMENT` in the handler; the plain-decrement branch in
  `ReserveStockIdempotent` and the legacy `ReserveStock` repo method are deleted once grep confirms no other caller (bundle code
  does not call it). Every decrement therefore has a ledger row.
- Why scope and not just NetworkPolicy (ADR-0010): the gateway is an *allowed* caller of the domain and exposes these RPCs;
  NetworkPolicy cannot distinguish an anonymous user's forwarded call from a legitimate one. The gate relies on the gateway
  rebuilding `x-principal-*` each hop (AGENTS.md section 4); an e2e scenario sends a forged `x-principal-scopes: inventory.write` header through
  the gateway to prove it is not honoured. Direct in-cluster access with forged metadata remains the ADR-0010 gap (mTLS follow-up).

*Alternatives:* (a) gate on `principal.type == SERVICE` alone: a forged `x-principal-type` is as easy as a forged scope, and a
scope is what `RequireScopes` already models; (b) remove the routes from the gateway: right as defense in depth, but it is a
gateway change, recorded as a non-goal/open question; (c) a shared secret header between team-order and team-domain:
reintroduces the anti-pattern ADR-0006 removed.

### D10 — Configuration and dedupe fixes (#8, #9)

- Correct the tag to `DB_MAX_CONNS`; add a `Kafka` settings group (`KAFKA_ENABLED`, `KAFKA_BROKERS`,
  `ORDER_PAYMENT_CONSUMER_GROUP`, `PAYMENT_EVENTS_TOPIC`, `PAYMENT_EVENTS_DLQ_TOPIC`, same defaults) and read it from
  `bootstrap/kafka.go` instead of ad hoc `envStr/envBool/envList`; add `RESERVATION_TTL` (Go duration, default 15m, invalid or
  non-positive falls back with a warning, same shape as `RESERVATION_SWEEP_INTERVAL`) wired to `WithReservationTTL`.
- Port `CheckEnvExample` from team-domain (`envcheck.go` + `TestEnvExampleInSync`): it parses uncommented `KEY=` lines, so the
  five Kafka keys and `RESERVATION_TTL` must be **uncommented** in `.env.example` (their defaults are the code defaults, so behaviour is unchanged).
  README table updated (the "`DB_MAX_CONns`" caveat is deleted). team-order has no Makefile; the test runs under
  `go test ./...`, and a `check-env` target is optional.
- `IsProcessed`: `errors.Is(err, pgx.ErrNoRows)` -> `(false, nil)`; any other error -> `(false, err)`. The consumer already
  converts that to a retryable error. The PENDING-only compare-and-set still prevents a double apply.

### D11 — Order-stable, per-aggregate outbox delivery (#10)

- **Schema.** Migration `team-domain/0011` (and `team-payment/0005`): `ADD COLUMN seq BIGINT GENERATED ALWAYS AS IDENTITY` on
  the outbox table plus a partial index `(seq) WHERE status = 'pending'` and `(aggregate_id, seq) WHERE status = 'pending'`. `seq`
  is assigned at `INSERT`, which happens **after** the business `UPDATE` holds the aggregate's row lock in the same tx, so for
  one aggregate `seq` follows the order changes were applied even when transactions overlap; `created_at` (tx start) cannot.
- **Claim.** `WITH candidates AS (SELECT ... WHERE status='pending' AND available_at <= now() AND (locked_until IS NULL OR
  locked_until <= now()) AND NOT EXISTS (older pending row of the same aggregate that is backed off or leased) ORDER BY seq
  FOR UPDATE SKIP LOCKED LIMIT $1), upd AS (UPDATE ... RETURNING ..., seq) SELECT ... FROM upd ORDER BY seq`.
  Only `pending` older rows block; a parked `failed` row does not (a poison event must not stall its aggregate).
- **Relayer.** On the first produce failure of an aggregate within a pass, skip that aggregate's remaining claimed events and
  call new `ReleaseClaims(ids)` (sets `locked_until = NULL`) so they are retried right behind it; other aggregates proceed.
- **Guarantee, stated precisely:** for one aggregate id, events are produced in `seq` order and a later event is never
  produced while an earlier one is pending-but-unpublished; delivery stays at-least-once (a crash after produce and before
  `MarkPublished` redelivers the earlier event, possibly after later ones were produced - consumers keep deduping on `event_id`
  and self-diffing); no ordering across aggregates.
- **Test that fails on the old code** (Postgres via `TEST_DATABASE_URL`, skip convention of `pg_testutil_test.go`): insert E1 then E2
  for one aggregate, `UPDATE` E1 (its new tuple now sits after E2 in the heap) and claim: old code returns E2, E1; new code E1, E2.
  Relayer tests use the existing fake store/producer. `team-payment` has no Postgres test helper; task adds one.
- `team-payment` has the identical query (`payment_outbox_events`); aggregate is `order_id`. Same change, same tests.

*Alternatives:* (a) Go-side `sort.Slice` by `created_at` after claim: fixes the evidence case but not tx-start inversion or
backoff overtake, and hides the order guarantee outside SQL; (b) `ORDER BY created_at, event_id` in an outer query only: same
weakness; (c) one relayer per aggregate / Kafka transactions: disproportionate.
Cost: a column add (identity backfill rewrites the table) on an outbox that also holds published rows; do it off-peak and prune
published rows first if the table is large.

### D12 — Grant `recommendations:read`; scope-drift test (#11)

Add `recommendations:read` to buyer, seller and admin in `roleScopes` (precedent: commit `98f0b31` for `search:write`).
Mismatch survey of every scope a service enforces vs identity's grants: `listing.read|write`, `search:read|write`,
`engagement:read|write` are granted as intended; **`recommendations:read` was the only enforced scope granted to nobody**.
Observations, not changes here: anonymous `PUBLIC_SCOPES` (gateway) lacks `recommendations:read`, so anonymous Recommend is also
403 (gateway config, other change); `isAdminOrUser` tolerates two never-issued admin scopes. A test in `team-identity`
declares the enforced-scope list (with the owning service) and fails if any is granted to no intended role, and asserts
`inventory.write` is in none. Existing sessions keep their old scope set until login/expiry (TTL 1h).

### D13 — E2E approach

All API level through the gateway (Connect JSON), one `.feature` per capability under `platform-e2e/tests/e2e/features/`:
#1 stock unchanged after short order/domain TTLs and a sweep pass (needs `RESERVATION_TTL` for both services in the e2e compose),
#2+#4 two-seller cart with one item over stock -> error, no orders, cart intact; seller raises stock; unkeyed retry succeeds,
#3 voucher order cancelled then re-`ValidateAndReserve` with the order id answers "already released", #7 anonymous/buyer/forged-header
`ReserveStock` rejected and stock unchanged, #10 the existing price-drop scenario repeated in a loop (create then immediate price cut),
#11 buyer `Recommend` is not `insufficient_scope`. Fault-injection cases (failed binding, ambiguous commit, repair) are
covered by service-level tests, not e2e.

## Risks / Trade-offs

- [Deploy order: team-domain enforcing `inventory.write` before every team-order pod sends the service principal fails checkouts
  with `PERMISSION_DENIED`] -> deploy team-order fully first, then team-domain; do not roll team-order back below the
  service-principal version while the gate is live; rollback of the gate is a redeploy of the previous domain image.
- [`PlaceOrders` holds one transaction across N orders] -> N is the number of sellers in one cart (small); no network calls
  inside the tx (all remote calls finish in Phase A/B).
- [Holding all sellers' stock until Phase C] -> same TTL as today; checkout latency is dominated by the same RPCs, now
  sequenced as A then B instead of interleaved.
- [Sweep repair binds a reservation to a live order; a wrongly-set intended id could bind to the wrong order] -> the intended
  id is generated and written by the same code path as the order; repair only binds when `order.buyer_id == reservation.buyer_id`
  (checked in repair, task 7.2).
- [Unkeyed retries now double-hold stock until the abandoned attempt settles] -> bounded by one TTL; clients that care use a key.
- [Truthful saga view shows `SKIPPED`/`PENDING` where the old view showed green] -> intended; `paid_at` is NULL for orders paid
  before the migration, so those show payment `SUCCESS` without a time, and a legacy paid-then-cancelled order shows `SKIPPED`.
- [Voucher commit-before-CAS race on cancel vs payment] -> documented residual (D6).
- [Outbox `seq` migration rewrites the table] -> off-peak, prune published rows; additive and reversible (`DROP COLUMN`).
- [Existing tokens lack `recommendations:read` for up to an hour] -> expected; no action needed beyond a re-login.
- [A scope gate can only trust the gateway's rebuilt metadata] -> e2e forged-header scenario; mTLS (ADR-0010 follow-up) remains the real fix.

## Migration Plan

1. `platform-core`: ADR-0007/0008/0010 amendments and proto **comment** edits (no shape change; `buf lint`/`buf breaking` clean). No runtime effect.
2. `team-identity` (independent): ship the scope grants; e2e for #11 can run as soon as this is live.
3. `team-order` migration `0007` (`paid_at`), then `team-order` code: service principal for stock RPCs first (harmless to an
   un-gated domain), plus D1-D7, D10. Run the drift query (task 7.5) after deploy.
4. `team-domain`: migration `0011` (outbox `seq`), then code (scope gate, empty-id rejection, ordered claim). Only after step 3 is 100% rolled out.
5. `team-payment`: migration `0005`, then code.
6. Rollback: team-domain gate and outbox change roll back independently (migrations are additive); team-order can roll back
   only while the domain gate is off or before step 4; the `paid_at` column is left in place.

## Open Questions

- Should a committed voucher redemption be refunded when a **Paid** order is cancelled (needs a team-promotion semantics change:
  `ReleaseReservation` of a committed hold decrementing `used`, or a new RPC)? Default here: not refunded, documented.
- Should `GetShipmentTracking` by **tracking code** stay available without login for a public tracking link? Default here: no (login + ownership).
- Should the gateway stop routing `ReserveStock`/`ReleaseStock` at all (defense in depth)? Default here: handled in a separate gateway change.
