## Context

See `proposal.md` — *Why*. Current state relevant to the approach (verified in code):

- Statuses are `INT` (`Pending=1, Paid=2, Shipped=3, Completed=4, Cancelled=5`) in `orders.status` with no
  CHECK. `OrderRepository.UpdateOrderStatus(id, status, tracking)` runs an unconditional
  `UPDATE orders SET status=$2 WHERE id=$1`. Callers: handler `UpdateOrderStatus`, service `CancelOrder`,
  service shipment creation (`_, _ =` discards the error), and the `PaymentSettled` consumer, which reads the
  order, checks `Pending`, then writes `Paid`.
- `ErrInvalidStatus` is defined in the service but the handler maps any `CancelOrder` error to `INTERNAL`.
  `UpdateOrderStatus`'s handler allows buyer *or* seller with no per-transition rule.
- `CreateOrder` creates an `order_sagas` row per call with a random id; reservation ids come from
  `ReservationID(buyerID, cartItem)` which is stable per cart item, so a retry reuses the id and
  `ReserveStockIdempotent` returns success without decrementing. `RemoveItems` failure is logged only.
- `team-gateway` rebuilds gRPC metadata for each hop (`outgoing`): principal headers plus request id, and
  nothing else, so a client header is dropped unless explicitly forwarded.
- Config has `Runtime.Env` (env var `ENV`, default `local`) and an `IsProd()` helper that matches `prod` /
  `production` only. `main.go` picks repositories by `res.Pool != nil`; the saga repo, consumer and sweeper
  are gated on the pool. `DATABASE_ENABLED` defaults to `true`.
- Constraints: Rule 1 (frontend only via gateway), Rule 2 (gateway no business logic), Rule 3 (own DB),
  Rule 4 (contract in platform-core, no forks), ADR-0007 (durable saga), ADR-0008 (reservations).

## Goals / Non-Goals

**Goals:**

- No order status change that the business did not define can be stored, by any caller or race.
- Exactly one of any set of concurrent conflicting transitions wins; losers get a typed, non-retryable error.
- A retried or double-submitted checkout never produces a second order or a second charge.
- A misconfigured staging/production instance refuses to boot instead of running on memory.

**Non-Goals (design-level):** no event sourcing of status history, no `order.events` (ADR-0012), no state
machine for returns/shipments, no refund workflow, no proto change.

## Decisions

### D1 — Transition table in the service, enforced by an atomic conditional UPDATE (vs. read-check-write, row locks, or a DB trigger)

One table in `service` maps `to -> allowedFrom` (plus which actor class may perform it). The repository gains
`UpdateOrderStatusFrom(ctx, id, to, allowedFrom []OrderStatus, tracking)` implemented as
`UPDATE orders SET status=$2 [, tracking number when given, as today] WHERE id=$1 AND status = ANY($3) RETURNING ...` (the SET list otherwise mirrors the existing statement).
Zero rows: a follow-up `SELECT` distinguishes `ErrOrderNotFound` from a typed `ErrStatusConflict`, which the
service wraps as `ErrInvalidStatus` and the handler maps to `FAILED_PRECONDITION`. The in-memory repo does the
same under its mutex so tests exercise the same contract.
*Alternatives:* `SELECT ... FOR UPDATE` then update (works, but holds a transaction across service code and
still needs the same predicate to be safe against other writers); a version/`updated_at` optimistic lock (what
the vault note suggests; equivalent here because the status itself is the version that matters, and it avoids
adding a column); a DB trigger enforcing transitions (strongest but hides the rules from code review and
cannot express the actor rule). The conditional UPDATE is one statement, needs no new column, and is the
"optimistic lock + FSM" shape with the expected status as the lock value.

Callers that previously used the unconditional `UpdateOrderStatus` (consumer, shipment) move to the same
conditional method; the unconditional method is removed from the interface so a new caller cannot bypass the
guard.

### D2 — Cancel claims first, then releases (vs. release then flip)

`CancelOrder` runs `UpdateOrderStatusFrom(..., Cancelled, [Pending, Paid])`. The caller that gets a row back is
the winner and only then releases the order's reservations (by their original `reservation_id`s, from
`order_reservations` by `order_id`); failures are parked as `RELEASE_FAILED` for the existing sweep.
*Alternative:* release first, flip last (what `inventory-commit-and-idempotent-release` D4 describes). That is
unsafe alone: between the status read and the flip the order can become `Shipped`, and stock would already
have been given back for an order that ships. Claim-first makes the status decision authoritative before any
side effect. The cost is a crash window (order `Cancelled`, reservations still `COMMITTED`); the sweep gains
one query, "reservations of `Cancelled` orders that are not released", so the window is bounded by the sweep
interval instead of being permanent.

This change depends on release being idempotent on `reservation_id` (sibling change): with a blind release a
retry of the post-claim release would double-restore. Until that lands, the winner-only rule still removes the
concurrent double-release, but sweep re-release and retries are only safe after the sibling change.
Sequencing is therefore: sibling change first, or this change's cancel task (4.4) after it.

### D3 — Idempotency-Key as gRPC metadata, not a proto field (vs. `idempotency_key` on `CreateOrderRequest`)

The client sends an `Idempotency-Key` HTTP header; the gateway copies it into outgoing metadata
(`idempotency-key`) next to the principal metadata; `team-order` reads it from incoming metadata.
*Alternatives:* a proto field `string idempotency_key = 5`. Additive and `buf breaking`-safe, and more
discoverable, but it is a cross-service contract change that must be merged, re-vendored and regenerated in
the gateway, frontend and order before anything works, and the key is transport concern, not part of what a
checkout *is*. Metadata needs no contract change, matches the HTTP convention (`Idempotency-Key` header,
IETF draft), and keeps Rule 2 intact: the gateway only forwards a header, exactly like the request id. If a
future non-HTTP caller needs it, adding the field later is still additive. The gateway change is one line in
`outgoing` restricted to `CreateOrder`, and the value is validated only by `team-order` (length, printable).

### D4 — Key stored on the saga row, unique per buyer; replay reads the saga's orders

Migration adds `order_sagas.idempotency_key TEXT NULL` and
`CREATE UNIQUE INDEX ... ON order_sagas (buyer_id, idempotency_key) WHERE idempotency_key IS NOT NULL`.
`CreateSaga` becomes insert-or-lookup: `INSERT ... ON CONFLICT DO NOTHING RETURNING`; on conflict the existing
saga is read and handled by status:

- `COMPLETED`: load `order_reservations` by `saga_id`, collect their distinct `order_id`s, return those orders;
  re-attempt the cart clear (best effort) so a prior `RemoveItems` failure self-heals.
- `PENDING` (first attempt still running or crashed): return `ABORTED` ("checkout in progress, retry"). A
  crashed saga is settled by the existing sweep (reservations released, saga compensated).
- `COMPENSATED` / `FAILED`: the saga released its reservations; it clears `idempotency_key` (set `NULL`) as part
  of compensation so the key is reusable and the retry runs as a fresh saga.

`ReservationID` is derived from the key when present (`buyerID | key | cartItemID | ...`) so a re-executed
saga after compensation still reserves under ids that cannot collide with a replay of a different key, and two
different keys never share a reservation row. Without a key the current derivation is unchanged.
*Alternatives:* a separate `idempotency_keys` table with a response blob (more general, needs TTL cleanup and
stores a serialized response that can go stale); add `saga_id` to `orders` (cleaner join but rewrites the
hot table; the reservation-to-order link already exists). Storing on the saga reuses the durable header
that ADR-0007 already mandates.

A missing key keeps today's behaviour so the seed tools and non-UI callers keep working; the frontend always
sends one. Making the key mandatory is a possible later step once callers are migrated.

### D5 — Cart clearing is decoupled from order identity

Because a replay returns the original orders (D4), a failed `RemoveItems` no longer matters for correctness:
the next attempt with the same key repairs the cart, and an attempt with a *new* key against a still-full
cart is a deliberate new checkout by the user (the cart UI shows the items). The failure stays logged and
gains a metric.

### D6 — Stale payment events are ignored by the same conditional write

The consumer calls `UpdateOrderStatusFrom(..., Paid, [Pending])`. `ErrStatusConflict` is not an error for the
stream: log `order_id` and current status, count `order_payment_event_ignored_total`, acknowledge the event
(retrying cannot help). The existing "already past PENDING" read-check stays as a cheap short-circuit but is no
longer the safety mechanism. A late payment on a `Cancelled` order leaves money taken with no order; the
refund is explicitly a follow-up (open question), but the log makes it findable.

### D7 — Actor rule lives in the handler/service boundary

The handler resolves the actor class from the principal: `buyer` (order's `BuyerID`), `seller`
(`SellerID`), `system` (consumer / shipment, internal calls, not from an RPC principal). The service's table
carries, per transition, the set of actor classes allowed; the buyer is allowed only `-> Cancelled`. Checking
actor and status together in the service keeps it unit-testable without gRPC. A mismatch is
`PERMISSION_DENIED`, a status mismatch is `FAILED_PRECONDITION`; actor is checked first so a stranger learns
nothing about the order's status.

**Decided after auth review:** `Pending -> Paid` is `system` only (the PaymentSettled consumer) — a seller must
not be able to mark their own order paid, then ship and complete it. `CreateOrder` rejects buyer == seller
(`FAILED_PRECONDITION`, before any reservation), which also removes the case where `orderActor` would resolve a
self-buyer as seller; for legacy self-orders `orderActor` prefers `buyer`.

### D8 — DB CHECK on status, added `NOT VALID` then validated

`CHECK (status BETWEEN 1 AND 5) NOT VALID`, then `VALIDATE CONSTRAINT` in a separate step after a count query
shows no violations. It is a backstop against a future code path writing a raw value, not the enforcement
mechanism (D1 is). Transition rules are not expressed in SQL (see D1 alternatives).

### D9 — Fail fast is a boot-time check on `ENV`, not a fallback flag

After config load and before repositories are chosen, `main` calls `settings.RequireDurableStorage()`:
if `ENV` (normalised, lowercase) is `staging`, `stage`, `prod` or `production` and either
`Database.Enabled` is false or `res.Pool == nil`, return an error naming `ENV` and `DATABASE_ENABLED`;
`main` logs it and exits non-zero before opening the gRPC port. For `local` and `test` the in-memory branch runs and logs a warning that data is not durable. Unknown values
are treated as non-strict to match today's `local` default; the error message lists the strict values.
The repository selection in `main.go` stays as is; only the guard is added, so the local developer flow is
unchanged.
*Alternative:* delete the in-memory repos. Rejected: unit and e2e-in-process tests rely on them.
*Note:* the variable is `ENV` in this service, not `APP_ENV`; the guard uses what the config already reads.

## Risks / Trade-offs

- [Existing clients or seed scripts relied on the unguarded `UpdateOrderStatus` (for example seeding a
  `Completed` order directly)] → the seed and the e2e helpers are checked in task 2.x; seeding moves to legal
  paths (create, pay, ship, complete) or an internal test-only repository hook, not an RPC bypass.
- [`orders` rows with an out-of-set status block the CHECK] → `NOT VALID` first, validate after a count query.
- [Claim-then-release leaves a crash window] → bounded by the sweep extension (D2); covered by a test that
  cancels, simulates failing release, and asserts the sweep releases.
- [Sibling change not landed yet: release is not idempotent] → sequencing note in D2 and the task order; the
  cancel task states the dependency and its test uses a fake that dedupes by reservation id.
- [Replaying the orders of a `COMPLETED` saga when `order_reservations` rows were cleaned up] → the sweep does
  not delete rows; a missing set returns `ABORTED`/`INTERNAL` with a log rather than creating a new order.
- [Key reused with a *different* cart or address] → the replay returns the original orders (documented HTTP
  idempotency semantics); the frontend generates a new key per attempt, and only reuses it for the same
  attempt, so this is a client bug, not a server ambiguity. No request fingerprint in this change.
- [Metadata key dropped by an intermediate proxy] → the gateway test asserts forwarding; absence degrades to
  today's behaviour, not to an error.
- [Fail-fast could take down a staging env that has been silently in-memory] → intended; call it out in the
  release note and confirm the staging `DATABASE_URL` before deploy.

## Migration Plan

1. `team-order` migration `0006`: `orders.status` CHECK `NOT VALID`; `order_sagas.idempotency_key` + partial
   unique index. Both additive. Run `VALIDATE CONSTRAINT` after a data query.
2. `team-order` code: conditional repository write and transition table, handler mapping, consumer, shipment,
   cancel, idempotent `CreateOrder`, boot guard. Deploy.
3. `team-gateway`: forward `Idempotency-Key` for `CreateOrder`. Deploy. (Safe before or after step 2: order
   ignores a missing key.)
4. `team-frontend`: generate and send the key. Deploy after step 3.
5. Before enabling the boot guard in an environment, confirm `ENV` and `DATABASE_ENABLED`/`DATABASE_URL`
   values there.
6. Rollback: frontend/gateway independently (key simply stops being sent); `team-order` rollback leaves the
   additive columns and the NOT VALID/validated constraint in place, which old code never violates.

## Open Questions

- Should a late `PAYMENT_SUCCESS` on a `Cancelled` order trigger an automatic refund through `team-payment`?
  (Out of scope here; the ignored-event log and metric make the cases countable.)
- Does the product want sellers to cancel a `Paid` order, or only buyers? The table allows both for
  `Pending` and `Paid -> Cancelled`, matching today's handler, which accepts either party.
- Should a key become mandatory once the seed tools and the `platform-e2e` API helpers send one?
