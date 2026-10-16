## Why

An online-pay order that is never paid stays PENDING forever: it keeps its domain stock reservation, its voucher hold and its flash-sale units (a placed flash order counts as sold until cancelled, accepted in Q5), and the buyer is never told (backlog Q20, `plans/pm-review-2026-10/buyer-journey.md` #6, `trust-ops.md` #7). The locked initiative (`plans/q10-order-lifecycle/00-spec.md`, 2026-10-16) gives every online-pay order a 24-hour payment deadline, a reminder 2 hours before it, and a system cancel after it that releases everything through the existing buyer-cancel path. The cancel also stores a reason code (`CancelReason`), so the system cancel (`PAYMENT_EXPIRED`), the buyer cancel and the admin force-fail are told apart on the order and on `OrderCancelled`. This slice is the team-order side of that: payment deadline snapshot, `OrderPlaced` and `OrderPaymentReminder` events, the lifecycle sweep (reminder and expiry passes), the Pending to Cancelled system transition, and the reason code. The proto comes from `q10-order-lifecycle-contract`; the refund of a late payment is `payment-unpaid-cancel`; the receipt, completion, return and ship-by passes of the same sweep are `order-receipt-completion`.

initiative: q10-order-lifecycle

## What Changes

- **Migration `0008_order_payment_deadline`** (additive, re-runnable, like 0007): `orders.payment_due_at TIMESTAMPTZ`, `orders.payment_reminder_sent_at TIMESTAMPTZ`, `orders.cancel_reason INT NOT NULL DEFAULT 0`, plus a partial index on `(payment_due_at) WHERE status = 1 AND payment_due_at IS NOT NULL` for the sweep. No backfill (human decision B7): an unpaid order placed before the migration has no `payment_due_at` and is never auto-expired.
- **Config** (`internal/config`): `PAYMENT_DEADLINE` (default `24h`), `PAYMENT_REMINDER_BEFORE` (default `2h`), `LIFECYCLE_SWEEP_INTERVAL` (default `1m`). Same accessor pattern as `ReservationTTL`: an empty, unparsable, zero or negative value falls back to the default and returns a warning for the caller to log; boot never fails.
- **Placement** (`internal/service/order.go`, `internal/repository/placer.go`, `order.go`):
  - `Order` gains `PaymentDueAt *time.Time`, `PaymentReminderSentAt *time.Time`, `CancelReason` and the matching columns in `orderColumns`, `scanOrder`, `insertOrderTx` and the in-memory repository.
  - For a payment method other than COD, `payment_due_at = created_at + PAYMENT_DEADLINE`, read from config at placement and stored; for COD it is NULL.
  - Each order inserted by `PlaceOrders` (Postgres and in-memory placer) writes one `OrderPlaced` outbox row in the same transaction (event id deterministic per order id). The `ship_by` field of `OrderPlaced` is left unset here; `order-receipt-completion` sets it for COD.
- **Transition table** (`internal/service/transitions.go`): `Pending -> Cancelled` gains the actor `ActorSystem` (expiry only). `cancelFrom` for the buyer stays `Pending, Paid`; the system expiry uses `AllowedFrom(Cancelled, ActorSystem)` = `Pending`.
- **Cancel path** (`internal/service/order.go`, `internal/repository/order.go`, `internal/handler/order.go`, `internal/events/publisher.go`):
  - `CancelOrder` takes a `CancelReason` and an actor. The compare-and-set `UpdateOrderStatusFrom` also writes `cancel_reason` when moving to Cancelled, and `BuildCancelledOutboxRow` sets `OrderCancelled.reason_code` from the order row (same transaction as today).
  - The `CancelOrder` handler reads `reason_code` from the request. A buyer sending `CANCEL_REASON_PAYMENT_EXPIRED` or `CANCEL_REASON_ADMIN_FORCE_FAIL` gets `PERMISSION_DENIED` before any write. `ForceFailSaga` passes `CANCEL_REASON_ADMIN_FORCE_FAIL`. An unset code is stored as `CANCEL_REASON_UNSPECIFIED` (older clients keep working).
  - The claim-then-release code after the claim (reservations, voucher, flash holds) is extracted unchanged into one method used by the buyer cancel, `ForceFailSaga` and the expiry, so there is exactly one release path.
  - `toWireOrder` returns `payment_due_at` and `cancel_reason`.
- **Lifecycle sweep** (`internal/service/lifecycle.go` new, `cmd/server/main.go`): `OrderService.SweepLifecycle(ctx, now)` runs the passes in order, each bounded to 100 rows and independent (an error in one pass is logged and the next pass still runs):
  1. Expiry: Pending orders with `payment_method <> COD`, `payment_due_at < now`: system cancel with `PAYMENT_EXPIRED`.
  2. Reminder: Pending orders with `payment_method <> COD`, `payment_reminder_sent_at IS NULL`, `payment_due_at - PAYMENT_REMINDER_BEFORE <= now < payment_due_at`: one compare-and-set that stamps `payment_reminder_sent_at` and writes one `OrderPaymentReminder` outbox row in the same transaction.
  `order-receipt-completion` appends its passes to this same function. `runLifecycleSweeper` in `cmd/server/main.go` mirrors `runReservationSweeper` on `LIFECYCLE_SWEEP_INTERVAL`.
- **Consumer** (`internal/consumer/payment.go`): no code change. A `PaymentSettled` for a Cancelled order is already acknowledged without change and never commits the voucher; this slice adds a regression test.
- **Proto**: `team-order/proto` copies refreshed from `platform-core/packages/proto` (contract slice) and Go stubs regenerated; never hand-edited.
- **platform-e2e** (owned by this slice): `compose/order-inventory.override.yaml` gains short lifecycle timers for team-order; new `features/order/unpaid_expiry.feature` and steps; helpers in `support/`.
- Not changed: team-payment (voids the transaction and refunds, `payment-unpaid-cancel`), team-notification, team-gateway (`CancelOrder` route exists; the request field passes through), the frontend, team-domain, team-promotion.

## NFR

- Timeliness: a reminder or an expiry fires within one `LIFECYCLE_SWEEP_INTERVAL` (default 1 min = 60 s) of its time. In the e2e overlay the interval is 2 s.
- Bounds: each pass handles at most 100 orders per tick; a backlog drains over successive ticks (100 per tick per pass). Each expiry uses the same inline release budget as a buyer cancel; a release that cannot finish is parked `RELEASE_FAILED` and retried by `SweepExpiredReservations`.
- Idempotency: the expiry claim and the reminder stamp are compare-and-set writes; a repeated or concurrent sweep (two replicas) produces one `OrderCancelled` and one `OrderPaymentReminder` per order. Event ids are deterministic per order id.
- Availability: a failing pass or a failing upstream release never stops the other passes, the reservation sweeper or checkout. An expired order whose release was parked is already Cancelled.
- Limits: amounts are int64 VND; no new money path. Durations are env config (defaults above) and are snapshotted into `payment_due_at` at placement.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `orders.payment_due_at`, `payment_reminder_sent_at` | operational timestamps, pseudonymous (order id) | deadline of an unpaid order, once-only reminder | contract performance (order processing) | with the order row (existing order retention) |
| `orders.cancel_reason` (enum) | operational, not free text | tell buyer, system and admin cancels apart | same | with the order row |
| `OrderPlaced`, `OrderPaymentReminder`, `OrderCancelled.reason_code` on `order.events` | operational: ids, amount, currency, timestamps | notifications and refund | same | Kafka topic retention (existing); outbox rows follow the existing outbox retention |

No new personal data: events carry buyer id, seller id, order id, amounts and timestamps only, never an address or a name. `cancel_reason` is a closed enum; the free-text label stays client-side.

## Threat model

- **Assets:** order state, held stock and vouchers, flash units, the buyer's money (indirectly, through the refund in `payment-unpaid-cancel`).
- **Abuse cases:**
  - A buyer cancels with a system reason code to hide a self-cancel or to spoof an expiry: `CancelOrder` refuses `PAYMENT_EXPIRED` and `ADMIN_FORCE_FAIL` from a buyer with `PERMISSION_DENIED`, before any write.
  - A buyer or seller forces an early expiry: the expiry only runs from the sweep (no RPC) and only for orders whose stored `payment_due_at` has passed.
  - A config typo (zero or negative deadline) cancelling every new order at once: an unusable value falls back to the default with a warning.
  - Expiry racing a payment: both are compare-and-set from Pending, so the order is either Paid or Cancelled; a payment that settles after the cancel changes nothing here and is refunded by team-payment.
  - Two replicas sweeping at once: the claim is a single-row compare-and-set; only the winner releases and emits.
  - A COD order expiring by mistake: the sweep filters `payment_method <> COD` and a COD order has no `payment_due_at`.
  - Reminder spam: `payment_reminder_sent_at` is stamped in the same transaction as the event; a second sweep emits nothing.
- **Authz rule:** `CancelOrder` stays buyer-only (principal id = order buyer, else `PERMISSION_DENIED`); `ForceFailSaga` stays admin-only with both scopes; the system transition has no RPC, the sweep calls the service directly. No new RPC.
- **Accepted risk:** an online order placed before this migration has no deadline and stays PENDING until the buyer cancels or pays.
