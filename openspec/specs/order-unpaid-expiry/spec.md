# order-unpaid-expiry Specification

## Purpose
Unpaid online-pay orders get a payment deadline at placement, one reminder before it, and expire through the cancel path with a reason code.

## Requirements

### Requirement: Placement snapshots the payment deadline and emits OrderPlaced

For every order it places, `team-order` SHALL set `payment_due_at = created_at + PAYMENT_DEADLINE` (default 24h, read from config at placement and stored) when the payment method is not COD, and SHALL leave `payment_due_at` unset for a COD order. In the same transaction as the order insert, it SHALL write one `OrderPlaced` outbox row per order, keyed by order id, with `payment_due_at` unset for COD. A placement that rolls back SHALL write no `OrderPlaced`. `GetOrder` and the order lists SHALL return `payment_due_at`.

#### Scenario: Order expiry: Online-pay order gets a 24-hour deadline
- **WHEN** a buyer places an order with payment method MOCK_MOMO and `PAYMENT_DEADLINE` is 24h
- **THEN** `GetOrder` returns `payment_due_at` equal to `created_at` plus 24 hours (to the second) and one `OrderPlaced` outbox row exists for the order with the same `payment_due_at`
- **VERIFIED BY**: team-order `internal/repository` `TestPlaceOrdersSetsPaymentDueAtAndOrderPlaced`

#### Scenario: Order expiry: COD order has no deadline
- **WHEN** a buyer places an order with payment method COD
- **THEN** `GetOrder` returns no `payment_due_at` and one `OrderPlaced` outbox row exists whose `payment_due_at` is unset
- **VERIFIED BY**: team-order `internal/repository` `TestPlaceOrdersCodHasNoPaymentDueAt`

#### Scenario: Order expiry: Rolled-back placement emits no OrderPlaced
- **WHEN** `PlaceOrders` fails with `ErrReservationLost` because a reservation was swept
- **THEN** no order row and no `OrderPlaced` outbox row exist for the attempt
- **VERIFIED BY**: team-order `internal/repository` `TestPlaceOrdersRollbackWritesNoOrderPlaced`

#### Scenario: Order expiry: Each seller order of one checkout gets its own OrderPlaced
- **WHEN** a buyer checks out a cart with items from two sellers
- **THEN** two orders exist and exactly two `OrderPlaced` outbox rows exist, one per order id, each with the order's own seller_id and total_amount
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutEmitsOrderPlacedPerOrder`

#### Scenario: Order expiry: A later config change does not move an existing deadline
- **WHEN** an online-pay order was placed with `PAYMENT_DEADLINE` 24h and the service is restarted with `PAYMENT_DEADLINE` 1h
- **THEN** `GetOrder` still returns the original `payment_due_at`
- **VERIFIED BY**: team-order `internal/service` `TestPaymentDueAtSnapshotSurvivesConfigChange`

### Requirement: Lifecycle durations are env config with safe fallbacks

`team-order` SHALL read `PAYMENT_DEADLINE` (default 24h), `PAYMENT_REMINDER_BEFORE` (default 2h) and `LIFECYCLE_SWEEP_INTERVAL` (default 1m) as Go durations. An empty, unparsable, zero or negative value SHALL fall back to the default and produce a warning naming the variable; startup SHALL NOT fail.

#### Scenario: Order expiry: Defaults apply when nothing is configured
- **WHEN** none of the three variables is set
- **THEN** the deadline is 24h, the reminder offset is 2h, the sweep interval is 1m, and no warning is returned
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleDefaults`

#### Scenario: Order expiry: Invalid duration falls back with a warning
- **WHEN** `PAYMENT_DEADLINE` is "0", "-5m" or "soon"
- **THEN** the accessor returns 24h and a warning that names `PAYMENT_DEADLINE`, and the service still starts
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleUnusableDurationFallsBack`

### Requirement: An unpaid online-pay order is reminded once before its deadline

On each lifecycle sweep tick, `team-order` SHALL, for every order with status Pending, a payment method other than COD, no `payment_reminder_sent_at`, and `payment_due_at - PAYMENT_REMINDER_BEFORE <= now < payment_due_at`, stamp `payment_reminder_sent_at` and write one `OrderPaymentReminder` outbox row (order_id, buyer_id, total_amount, currency, payment_due_at, as in the `OrderPaymentReminder` contract) in the same transaction, with a compare-and-set on the unstamped Pending row. It SHALL write no reminder for an order that is Paid, Cancelled, COD, already reminded, or already past its deadline. A pass SHALL handle at most 100 orders per tick.

#### Scenario: Order expiry: Reminder is emitted two hours before the deadline
- **WHEN** a Pending MOCK_MOMO order reaches `payment_due_at - PAYMENT_REMINDER_BEFORE` (2h by default; 30s under the e2e overlay) and the next sweep tick runs
- **THEN** within one `LIFECYCLE_SWEEP_INTERVAL` and before `payment_due_at`, exactly one `OrderPaymentReminder` outbox row exists for the order with the order's `payment_due_at`, and the order's `payment_reminder_sent_at` is set
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: Reminder is emitted two hours before the deadline"

#### Scenario: Order expiry: Reminder boundary is inclusive at deadline minus offset
- **WHEN** the sweep runs one second before `payment_due_at - PAYMENT_REMINDER_BEFORE`, and again at exactly `payment_due_at - PAYMENT_REMINDER_BEFORE`
- **THEN** the first run emits no `OrderPaymentReminder` and the second emits one
- **VERIFIED BY**: team-order `internal/service` `TestReminderBoundary`

#### Scenario: Order expiry: Repeated sweeps never repeat the reminder
- **WHEN** the sweep runs three times inside the reminder window, and two replicas run it concurrently
- **THEN** exactly one `OrderPaymentReminder` outbox row exists for the order
- **VERIFIED BY**: team-order `internal/service` `TestReminderEmittedOnce`

#### Scenario: Order expiry: A paid order is never reminded
- **WHEN** the order moved to Paid before the reminder time and the sweep runs inside the reminder window
- **THEN** no `OrderPaymentReminder` outbox row exists for the order
- **VERIFIED BY**: team-order `internal/service` `TestReminderSkipsPaidOrder`

#### Scenario: Order expiry: A COD order is never reminded
- **WHEN** the sweep runs for a Pending COD order placed more than `PAYMENT_DEADLINE - PAYMENT_REMINDER_BEFORE` ago
- **THEN** no `OrderPaymentReminder` outbox row exists for the order
- **VERIFIED BY**: team-order `internal/service` `TestReminderSkipsCodOrder`

#### Scenario: Order expiry: A sweep that first sees an order past its deadline sends no reminder
- **WHEN** the first sweep that sees a Pending online order runs after its `payment_due_at`
- **THEN** the order is cancelled by the expiry pass and no `OrderPaymentReminder` outbox row exists for it
- **VERIFIED BY**: team-order `internal/service` `TestLateFirstSweepExpiresWithoutReminder`

#### Scenario: Order expiry: A reminder pass handles at most 100 orders per tick
- **WHEN** 250 Pending online orders are inside the reminder window and one sweep tick runs
- **THEN** 100 `OrderPaymentReminder` rows are written, and two more ticks write the remaining 150
- **VERIFIED BY**: team-order `internal/repository` `TestFindReminderDueBatchBound`

### Requirement: An unpaid online-pay order expires through the cancel path

On each lifecycle sweep tick, `team-order` SHALL, for every order with status Pending, a payment method other than COD and `payment_due_at < now`, move it Pending to Cancelled with the compare-and-set write (actor system, `cancel_reason` PAYMENT_EXPIRED), and only the sweep that wins the claim SHALL release the order's domain reservations, its voucher hold and its flash-sale units (committed units included) through the same release path as `CancelOrder`, and SHALL write one `OrderCancelled` (previous_status PENDING, reason_code PAYMENT_EXPIRED) in the claim's transaction. A COD order, an order without `payment_due_at`, and an order that is Paid, Shipped, Completed or Cancelled SHALL NOT be expired. An expiry whose release fails SHALL leave the order Cancelled with the release parked for the reservation sweeper. A pass SHALL handle at most 100 orders per tick.

#### Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
- **WHEN** a MOCK_MOMO order stays unpaid past `payment_due_at` and the sweep runs
- **THEN** within one sweep interval `GetOrder` returns status Cancelled with `cancel_reason` PAYMENT_EXPIRED, and the listing's available stock is back to its pre-checkout value
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: Order unpaid past its deadline is cancelled and its stock released"

#### Scenario: Order expiry: Expiry emits OrderCancelled with the expiry reason
- **WHEN** an unpaid online order is expired
- **THEN** exactly one `OrderCancelled` is published on `order.events` for the order with previous_status PENDING and reason_code PAYMENT_EXPIRED
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: Expiry emits OrderCancelled with the expiry reason"

#### Scenario: Order expiry: Expiry releases the voucher hold and flash units once
- **WHEN** an unpaid online order that holds a voucher reservation and a committed flash-sale line is expired and the sweep runs again
- **THEN** `ReleaseVoucherReservation` and `ReleaseFlashSaleStock` are each called once per hold id for the order, and the second sweep makes no release call
- **VERIFIED BY**: team-order `internal/service` `TestExpiryReleasesVoucherAndFlashOnce`

#### Scenario: Order expiry: Boundary at the deadline keeps the order payable
- **WHEN** the sweep runs at exactly `payment_due_at`, and again one second later
- **THEN** the order is still Pending after the first run and Cancelled after the second
- **VERIFIED BY**: team-order `internal/service` `TestExpiryBoundaryStrictlyAfterDeadline`

#### Scenario: Order expiry: A COD order is never expired
- **WHEN** a COD order is left Pending for longer than `PAYMENT_DEADLINE` plus one `LIFECYCLE_SWEEP_INTERVAL`, and a MOCK_MOMO control order placed at the same time is expired by the sweep
- **THEN** the COD order is still Pending and no `OrderCancelled` is published for it, while the control order is Cancelled with `cancel_reason` PAYMENT_EXPIRED
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: A COD order is never expired"

#### Scenario: Order expiry: An order without a deadline is never expired
- **WHEN** the sweep runs for a Pending online order placed before migration 0008 (`payment_due_at` NULL)
- **THEN** the order is still Pending
- **VERIFIED BY**: team-order `internal/service` `TestExpirySkipsOrderWithoutDeadline`

#### Scenario: Order expiry: A paid order is never expired
- **WHEN** an online order is Paid before its deadline and the sweep runs after the deadline
- **THEN** the order is still Paid and no `OrderCancelled` outbox row exists for it
- **VERIFIED BY**: team-order `internal/service` `TestExpirySkipsPaidOrder`

#### Scenario: Order expiry: Payment and expiry racing produce one outcome
- **WHEN** a `PaymentSettled` PAID and the expiry sweep act on the same Pending order after its deadline
- **THEN** exactly one compare-and-set wins; the order ends Paid with no `OrderCancelled`, or ends Cancelled with one `OrderCancelled`, never both
- **VERIFIED BY**: team-order `internal/repository` `TestExpiryVersusPaymentSingleWinner`

#### Scenario: Order expiry: Two sweeps expire an order once
- **WHEN** two lifecycle sweeps run concurrently over the same expired order
- **THEN** exactly one `OrderCancelled` outbox row exists and each reservation is released exactly once
- **VERIFIED BY**: team-order `internal/service` `TestExpiryConcurrentSweepsReleaseOnce`

#### Scenario: Order expiry: A failed release leaves the order cancelled and parked
- **WHEN** the domain `ReleaseReservation` call fails during an expiry
- **THEN** the order is Cancelled, its reservation is RELEASE_FAILED, and the next `SweepExpiredReservations` releases it
- **VERIFIED BY**: team-order `internal/service` `TestExpiryReleaseFailureParksForSweep`

#### Scenario: Order expiry: One failing pass does not stop the next
- **WHEN** the expiry pass (the first pass of the tick) returns a database error during a lifecycle sweep
- **THEN** the error is logged and the reminder pass of the same tick still runs and emits the `OrderPaymentReminder` due in that tick
- **VERIFIED BY**: team-order `internal/service` `TestSweepLifecyclePassesAreIndependent`

#### Scenario: Order expiry: An expiry pass handles at most 100 orders per tick
- **WHEN** 250 Pending online orders are past their deadline and one sweep tick runs
- **THEN** 100 orders are Cancelled after the tick and the remaining 150 are Cancelled after two more ticks
- **VERIFIED BY**: team-order `internal/repository` `TestFindExpiredBatchBound`

#### Scenario: Order expiry: A payment settling after the expiry changes nothing in team-order
- **WHEN** a `PaymentSettled` PAID event arrives for an order that was expired
- **THEN** the order stays Cancelled, its voucher is not committed, and the event is acknowledged
- **VERIFIED BY**: team-order `internal/consumer` `TestLateSettleAfterExpiryLeavesOrderCancelled`

### Requirement: Cancel stores a reason code and carries it on OrderCancelled

`CancelOrder` SHALL accept `reason_code` (`CancelReason`) and the winning compare-and-set SHALL store it in `orders.cancel_reason` and write it as `OrderCancelled.reason_code` from the claimed row. A buyer SHALL NOT send `CANCEL_REASON_PAYMENT_EXPIRED` or `CANCEL_REASON_ADMIN_FORCE_FAIL`: such a request SHALL get `PERMISSION_DENIED` before any write. `ForceFailSaga` SHALL cancel with `CANCEL_REASON_ADMIN_FORCE_FAIL`. An unset `reason_code` SHALL be stored as `CANCEL_REASON_UNSPECIFIED`. `GetOrder` SHALL return `cancel_reason`. The cancel authz is unchanged: only the order's buyer may call `CancelOrder`.

#### Scenario: Order expiry: Buyer cancel stores the chosen reason
- **WHEN** the buyer calls `CancelOrder` on their Pending order with `reason_code` CANCEL_REASON_CHANGED_MIND
- **THEN** `GetOrder` returns status Cancelled and `cancel_reason` CHANGED_MIND, and the `OrderCancelled` event carries reason_code CHANGED_MIND
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: Buyer cancel stores the chosen reason"

#### Scenario: Order expiry: Cancel without a reason code still succeeds
- **WHEN** the buyer calls `CancelOrder` on their Pending order without `reason_code`
- **THEN** the order is Cancelled with `cancel_reason` UNSPECIFIED and the stock is released
- **VERIFIED BY**: team-order `internal/handler` `TestCancelOrderWithoutReasonCode`

#### Scenario: Order expiry: Buyer cannot send a system reason code
- **WHEN** the buyer calls `CancelOrder` with `reason_code` PAYMENT_EXPIRED, and again with ADMIN_FORCE_FAIL
- **THEN** both calls fail with `PERMISSION_DENIED` and the order is still Pending
- **VERIFIED BY**: platform-e2e `features/order/unpaid_expiry.feature` "Order expiry: Buyer cannot send a system reason code"

#### Scenario: Order expiry: Another user cannot cancel with a reason
- **WHEN** a user who is not the order's buyer calls `CancelOrder` with `reason_code` CHANGED_MIND
- **THEN** the call fails with `PERMISSION_DENIED` and the order is unchanged
- **VERIFIED BY**: team-order `internal/handler` `TestCancelOrderOtherUserWithReasonDenied`

#### Scenario: Order expiry: Cancel of a shipped order is refused
- **WHEN** the buyer calls `CancelOrder` with `reason_code` CHANGED_MIND on a Shipped order
- **THEN** the call fails with `FAILED_PRECONDITION`, the order is still Shipped and `cancel_reason` is UNSPECIFIED
- **VERIFIED BY**: team-order `internal/service` `TestCancelShippedOrderKeepsReasonUnset`

#### Scenario: Order expiry: Admin force-fail records its own reason
- **WHEN** an admin calls `ForceFailSaga` on a Pending order
- **THEN** the order is Cancelled with `cancel_reason` ADMIN_FORCE_FAIL and `OrderCancelled.reason_code` ADMIN_FORCE_FAIL
- **VERIFIED BY**: team-order `internal/handler` `TestForceFailSagaStoresAdminReason`

#### Scenario: Order expiry: Concurrent buyer cancel and expiry keep one reason
- **WHEN** the buyer's `CancelOrder` (CHANGED_MIND) and the expiry sweep race on the same expired Pending order
- **THEN** exactly one wins; the stored `cancel_reason` equals the winner's and the single `OrderCancelled` carries the same reason_code
- **VERIFIED BY**: team-order `internal/repository` `TestCancelReasonMatchesClaimWinner`
