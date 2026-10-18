# unpaid-order-payment Specification

## Purpose
team-payment voids the payment of a cancelled unpaid order, refuses to settle it, and refunds once if it settles after the cancel.

## Requirements

### Requirement: A cancelled unpaid order's payment is voided

When team-payment consumes `OrderCancelled` with `previous_status` PENDING, the system SHALL set the order's payment transaction to `PAYMENT_STATUS_CANCELLED` if it is PENDING or FAILED, emit no `PaymentSettled` event for it, and leave an absent or already CANCELLED transaction unchanged.

#### Scenario: Pending transaction is voided on unpaid cancel
- **WHEN** `OrderCancelled` with previous_status PENDING and reason `PAYMENT_EXPIRED` arrives for order "O-1" whose transaction "P-1" is PENDING
- **THEN** "P-1" reads `PAYMENT_STATUS_CANCELLED` through GetPayment, no refund row exists and no `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingVoidsPendingTransaction`

#### Scenario: Failed transaction is voided on unpaid cancel
- **WHEN** `OrderCancelled` with previous_status PENDING arrives for order "O-2" whose transaction is FAILED
- **THEN** the transaction reads `PAYMENT_STATUS_CANCELLED`
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingVoidsFailedTransaction`

#### Scenario: Order without a transaction is a no-op
- **WHEN** `OrderCancelled` with previous_status PENDING arrives for an order that has no transaction (for example a COD order)
- **THEN** the record is committed, no transaction or refund row is created and nothing is parked in the DLQ
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingWithoutTransactionIsNoop`

#### Scenario: Redelivered cancel leaves one end state
- **WHEN** the same `OrderCancelled` (previous_status PENDING) is delivered twice for an order whose transaction was PENDING
- **THEN** the transaction reads CANCELLED, there are 0 refund rows, and the second delivery returns no error
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingRedeliveryIsIdempotent`

#### Scenario: Shipped-order cancel changes no transaction
- **WHEN** `OrderCancelled` with previous_status SHIPPED arrives for an order whose transaction is PAID
- **THEN** the transaction still reads PAID, no refund row is added and the record is committed without error
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledShippedLeavesTransactionUnchanged`

#### Scenario: Paid-order cancel still refunds without voiding
- **WHEN** `OrderCancelled` with previous_status PAID arrives for order "O-5" (amount 300000) whose transaction is PAID
- **THEN** exactly one refund row with key `cancel:O-5` and applied amount 300000 exists, the transaction reads REFUNDED and never CANCELLED
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPaidStillRefundsWithoutVoid`

#### Scenario: Malformed cancel is parked
- **WHEN** an `OrderCancelled` with an empty order_id arrives
- **THEN** the record goes to the DLQ as a permanent error and the offset is committed
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledMissingOrderIDIsPermanent`

### Requirement: A cancelled payment cannot be settled

`ProcessMockPayment` SHALL refuse a CANCELLED transaction, for both success and failure simulation, with `FAILED_PRECONDITION`, leaving the status CANCELLED, and SHALL keep checking buyer-or-admin first. Before a success settle it SHALL re-read the order from team-order (`GetOrder`): a CANCELLED order refuses the settle with `FAILED_PRECONDITION` "payment cancelled" and voids the PENDING or FAILED transaction to CANCELLED, and an unreachable team-order refuses with `UNAVAILABLE` without settling.

#### Scenario: Paying a cancelled transaction is refused
- **WHEN** the buyer calls ProcessMockPayment with simulate_success=true on a CANCELLED transaction
- **THEN** the call returns `FAILED_PRECONDITION` "payment cancelled", the transaction still reads CANCELLED and no `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/handler` `TestProcessMockPaymentCancelledRefused`

#### Scenario: Simulated failure on a cancelled transaction is refused
- **WHEN** the buyer calls ProcessMockPayment with simulate_success=false on a CANCELLED transaction
- **THEN** the call returns `FAILED_PRECONDITION` and the transaction still reads CANCELLED (not FAILED)
- **VERIFIED BY**: team-payment `internal/handler` `TestProcessMockPaymentCancelledFailureRefused`

#### Scenario: Non-owner is denied before the cancelled check
- **WHEN** a user who is neither the buyer nor an admin calls ProcessMockPayment on a CANCELLED transaction
- **THEN** the call returns `PERMISSION_DENIED`, not `FAILED_PRECONDITION`
- **VERIFIED BY**: team-payment `internal/handler` `TestProcessMockPaymentCancelledNonOwnerDenied`

#### Scenario: Settle that loses the race to the void is refused
- **WHEN** a success settle's compare-and-set finds the transaction CANCELLED because the void committed between its read and its write
- **THEN** the call returns `FAILED_PRECONDITION` "payment cancelled" and no `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/service` `TestSettleLosingToVoidIsRefused`

#### Scenario: Settle on an order already cancelled upstream is refused and voided
- **WHEN** the buyer calls ProcessMockPayment with simulate_success=true on PENDING transaction "P-4" whose order team-order GetOrder reports CANCELLED (the transaction was opened after the cancel event was consumed)
- **THEN** the call returns `FAILED_PRECONDITION` "payment cancelled", "P-4" reads `PAYMENT_STATUS_CANCELLED` and no `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/service` `TestProcessMockPaymentRereadsCancelledOrder`

#### Scenario: Settle is not attempted when team-order is unreachable
- **WHEN** the buyer calls ProcessMockPayment with simulate_success=true on a PENDING transaction and team-order GetOrder returns `UNAVAILABLE`
- **THEN** the call returns `UNAVAILABLE`, the transaction still reads PENDING and no `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/handler` `TestProcessMockPaymentOrderUnavailableNoSettle`

#### Scenario: Retry payment still works before the cancel
- **WHEN** a buyer's FAILED transaction on a PENDING order is re-settled with simulate_success=true before any cancel
- **THEN** the transaction reads PAID and one `PaymentSettled` outbox row is written
- **VERIFIED BY**: team-payment `internal/service` `TestFailedTransactionCanBeResettledBeforeCancel`

#### Scenario: Cancelled order cannot open a new transaction
- **WHEN** the buyer calls CreatePayment for a CANCELLED order that has no transaction
- **THEN** the call returns `FAILED_PRECONDITION` and no transaction row exists
- **VERIFIED BY**: team-payment `internal/handler` `TestCreatePaymentCancelledOrderRefused`

### Requirement: A payment that settles after the cancel is refunded once

If the order's transaction is PAID when an `OrderCancelled` with `previous_status` PENDING is applied (the settle won the race, or the void lost its compare-and-set), the system SHALL refund the transaction's full remainder exactly once under key `cancel:<order_id>`, write no seller credit and no seller deduction, and leave the transaction REFUNDED.

#### Scenario: Late payment is refunded in full
- **WHEN** order "O-3" (amount 500000) is settled PAID, then `OrderCancelled` with previous_status PENDING and reason `PAYMENT_EXPIRED` is applied
- **THEN** the transaction reads REFUNDED with refunded amount 500000, exactly one refund row with key `cancel:O-3` and applied amount 500000 exists, and the seller's ledger has 0 ORDER_SETTLEMENT and 0 REFUND_DEDUCTION entries
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingRefundsLateSettledPayment`

#### Scenario: Redelivery after a late-payment refund moves no money again
- **WHEN** the same cancel is delivered again after the refund of "O-3"
- **THEN** the refunded amount stays 500000 and the refund rows stay 1
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingLateRefundRedeliveryIsNoop`

#### Scenario: Void that loses the compare-and-set falls back to the refund
- **WHEN** the void finds 0 rows to update because the transaction became PAID between the consumer's read and its write
- **THEN** the consumer re-reads, refunds the full remainder once and ends with the transaction REFUNDED
- **VERIFIED BY**: team-payment `internal/service` `TestVoidLosingToSettleRefunds`

#### Scenario: Concurrent settle and cancel converge
- **WHEN** a success `ProcessMockPayment` and the cancel consumer run concurrently on a PENDING transaction, repeated 50 times
- **THEN** every run ends either CANCELLED with 0 refunds, or REFUNDED with exactly 1 refund of the full amount; never PAID, and never 2 refunds
- **VERIFIED BY**: team-payment `internal/service` `TestSettleAndCancelRaceConverges` (Postgres integration)

#### Scenario: Already refunded payment gets no second refund
- **WHEN** the transaction is already REFUNDED (full) when the PENDING-cancel is applied
- **THEN** no refund row is added and no error is returned
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingAlreadyRefundedIsNoop`

#### Scenario: Partially refunded late payment is refunded the remainder once
- **WHEN** order "O-6" (amount 500000) was settled PAID and partially refunded 200000, then `OrderCancelled` with previous_status PENDING is applied, and then delivered again
- **THEN** exactly one refund row with key `cancel:O-6` and applied amount 300000 exists, the refunded amount reads 500000 and the transaction reads REFUNDED after both deliveries
- **VERIFIED BY**: team-payment `internal/consumer` `TestCancelledPendingPartiallyRefundedRefundsRemainderOnce`

#### Scenario: Buyer cannot trigger the late-payment refund by RPC
- **WHEN** a buyer who is not the order's seller and holds no admin scope calls RefundPayment on the late-settled payment of a cancelled order
- **THEN** the call returns `PERMISSION_DENIED` and the refunded amount is unchanged
- **VERIFIED BY**: team-payment `internal/handler` `TestRefundPaymentBuyerDeniedOnCancelledOrderPayment`

### Requirement: An expired order cannot be paid end to end

An online-pay order cancelled by the system for non-payment SHALL show a CANCELLED payment and refuse any later pay attempt through the gateway.

#### Scenario: Expired order's payment is cancelled and cannot be paid
- **WHEN** a buyer places an online-pay order, opens its payment, and the order is cancelled by the payment deadline sweep (e2e overlay deadline of seconds)
- **THEN** within 30 s GetPayment for the order reads `PAYMENT_STATUS_CANCELLED` and a ProcessMockPayment with simulate_success=true returns `FAILED_PRECONDITION` and the order stays CANCELLED
- **VERIFIED BY**: platform-e2e `features/payment/unpaid_cancel.feature` `Expired order's payment is cancelled and cannot be paid`

#### Scenario: Buyer-cancelled pending order voids its failed payment
- **WHEN** a buyer's payment on a PENDING order fails (simulate_success=false) and the buyer then cancels the order
- **THEN** within 30 s GetPayment reads `PAYMENT_STATUS_CANCELLED` and a later pay attempt returns `FAILED_PRECONDITION`
- **VERIFIED BY**: platform-e2e `features/payment/unpaid_cancel.feature` `Buyer-cancelled pending order voids its failed payment`

#### Scenario: Another buyer cannot pay a cancelled order's transaction
- **WHEN** a different buyer calls ProcessMockPayment on the CANCELLED transaction of the expired order
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: platform-e2e `features/payment/unpaid_cancel.feature` `Another buyer cannot pay a cancelled order's transaction`
