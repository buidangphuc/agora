# return-refund-settlement Specification

## Purpose
Defines how an approved RMA return actually refunds the buyer. `team-order` records the return's refund as a durable
fact, and `team-payment` applies it once to the order's payment and the seller's ledger. A return can never take
back more than the order and the payment still allow.

## Requirements

### Requirement: Refunding a return records one ReturnRefunded fact in the same transaction

`UpdateReturnStatus` to `REFUNDED` SHALL succeed only for a return that is `APPROVED`, as a compare-and-set on that
status. In the same transaction it SHALL write one `ReturnRefunded` fact to `order.events` through `team-order`'s
outbox, keyed by the order id. The fact SHALL carry:

- the return id, the order id, the buyer and the seller;
- the `refund_amount` stored on the return (never a client-supplied amount);
- the order currency and the refund time.

Its event id SHALL be stable per return.

A return whose order was never paid online (the order has no paid time, e.g. cash on delivery) SHALL NOT be moved to
`REFUNDED`. The transition SHALL fail with `FAILED_PRECONDITION` and message `order was not paid online;
cash-on-delivery refunds are handled outside the system`, the return SHALL keep its status, and no fact SHALL be
written. Approving and rejecting such a return stay allowed.

A transition that is refused or lost to a concurrent transition SHALL write no fact. Concurrent refunds of one return SHALL leave exactly one winner, the others failing with
`FAILED_PRECONDITION`. Only the order's seller or an admin SHALL be able to make the transition. `team-order` SHALL
NOT call `team-payment` to refund.

#### Scenario: Refunding an approved return emits one ReturnRefunded with the stored amount

- **WHEN** a buyer's return of 200000 on a paid order of 500000 is approved by the seller and then moved to
  `REFUNDED` by the seller through the gateway
- **THEN** the return reads `REFUNDED`
- **AND** `order.events` carries exactly one `ReturnRefunded` record for that return, keyed by the order id, with
  `refund_amount` 200000

#### Scenario: Refunding a pending return is refused and emits nothing

- **WHEN** the seller moves a `PENDING` return straight to `REFUNDED`
- **THEN** the call fails with `FAILED_PRECONDITION` and the return stays `PENDING`
- **AND** `order.events` carries no `ReturnRefunded` record for it

#### Scenario: Concurrent refunds of one return emit one fact

- **WHEN** the seller sends eight concurrent `UpdateReturnStatus` calls moving one `APPROVED` return to `REFUNDED`
- **THEN** exactly one succeeds and the others fail with `FAILED_PRECONDITION`
- **AND** `order.events` carries exactly one `ReturnRefunded` record for that return

#### Scenario: Refunding a return on an order never paid online is refused

- **WHEN** a cash-on-delivery order handed over by its seller without an online payment has an `APPROVED` return of
  200000, and the seller moves it to `REFUNDED` through the gateway
- **THEN** the call fails with `FAILED_PRECONDITION` and message `order was not paid online; cash-on-delivery refunds
  are handled outside the system`
- **AND** the return still reads `APPROVED`
- **AND** `order.events` carries no `ReturnRefunded` record for it, and `order.events.payment-settlement.dlq` has no
  record for it

#### Scenario: The buyer cannot refund their own return

- **WHEN** the buyer of an order moves its `APPROVED` return to `REFUNDED` through the gateway
- **THEN** the call fails with `PERMISSION_DENIED`, the return stays `APPROVED`, and no `ReturnRefunded` record is
  written

### Requirement: team-payment refunds a return exactly once from its fact

On `ReturnRefunded`, `team-payment` SHALL refund the order's payment by the return's `refund_amount`, as one refund
with id `return:<return_id>`, source `RETURN` and reason `return_refunded`. That refund SHALL write the deduction for
the credited seller exactly as `seller-refund-deduction` requires. This SHALL hold:

- whichever of the return's refund and the order's credit is applied first;
- when `team-payment` was unavailable at the time of the refund;
- when the fact is redelivered or replayed (it then refunds and deducts nothing more);
- when one order has several returns, which refund cumulatively.

#### Scenario: An RMA refund refunds the buyer and deducts the seller

- **WHEN** the seller of a credited paid order of 500000 approves and refunds the buyer's return of 200000 through the
  gateway
- **THEN** within the settle window the buyer's `GetPayment` reads `PARTIALLY_REFUNDED` with a refunded amount of
  200000
- **AND** it lists one refund with source `RETURN` whose source id is the return id
- **AND** the seller has exactly one `REFUND_DEDUCTION` of -200000, referencing `return:<return_id>`

#### Scenario: An RMA refund while team-payment is stopped is applied when it restarts

- **WHEN** `team-payment` is stopped, the seller of a credited paid order of 500000 refunds an approved return of
  200000 through the gateway, and `team-payment` is started again
- **THEN** the return reads `REFUNDED` as soon as the call returns
- **AND** after the restart, within the settle window, the payment reads `PARTIALLY_REFUNDED` with a refunded amount
  of 200000, and the seller has exactly one `REFUND_DEDUCTION` of -200000 for the return

#### Scenario: A redelivered ReturnRefunded does not refund again

- **WHEN** a return's refund has been applied and its `ReturnRefunded` record is produced to `order.events` again,
  byte for byte
- **THEN** after a sentinel order of the same seller is credited, the payment still lists one refund for the return
  and has the same refunded amount
- **AND** the seller still has exactly one `REFUND_DEDUCTION` for the return

#### Scenario: Two returns on one order refund cumulatively

- **WHEN** a credited paid order of 500000 has two returns, of 200000 and 300000, and the seller approves and refunds
  the first and then the second
- **THEN** after the first the payment reads `PARTIALLY_REFUNDED` with 200000 refunded
- **AND** after the second it reads `REFUNDED` with 500000 refunded and two `RETURN` refunds
- **AND** the seller has two `REFUND_DEDUCTION` entries, -200000 and -300000, each referencing its return

#### Scenario: A return refund and a cancel of the same paid order refund the payment once in total

- **WHEN** the seller of a credited paid order of 500000 refunds an approved return of 200000, and the buyer then
  cancels the order
- **THEN** within the settle window the payment reads `REFUNDED` with a refunded amount of 500000
- **AND** it lists exactly one `RETURN` refund and one `ORDER_CANCEL` refund, whose applied amounts sum to 500000
- **AND** the seller's `REFUND_DEDUCTION` entries for the order sum to -500000

### Requirement: A return never refunds more than the order and the payment still allow

`team-order` SHALL refuse a `CreateReturnRequest` with `INVALID_ARGUMENT` when its refund amount exceeds the order's
returnable remainder: the order total minus the refund amounts of the order's returns that are not `REJECTED`.

- A request without an amount SHALL default to the remainder. A remainder of 0 SHALL refuse the request with
  `FAILED_PRECONDITION`.
- The check SHALL be serialised per order, so concurrent requests never exceed the order total.
- A rejected return SHALL free its amount for later returns.

When a `ReturnRefunded` asks for more than the payment's refundable remainder at the time it is applied (because a
seller or admin refund, or a cancellation, took part of it), `team-payment` SHALL refund only the remainder and record
the refund with the return's amount as requested and the remainder as applied. When nothing remains, the refund SHALL
be recorded with an applied amount of 0 and no deduction. Such a fact SHALL NOT be dead-lettered.

#### Scenario: A return above the order's remaining returnable amount is refused

- **WHEN** a paid order of 500000 has an open return of 300000 and the buyer requests a second return of 300000
- **THEN** the call fails with `INVALID_ARGUMENT`
- **AND** a second return of 200000 is accepted

#### Scenario: A rejected return frees its amount for a new return

- **WHEN** a paid order of 500000 has a return of 500000 that the seller rejects, and the buyer then requests a return
  of 500000
- **THEN** the new return is accepted as `PENDING`

#### Scenario: Concurrent return requests cannot exceed the order total

- **WHEN** the buyer of a paid order of 500000 sends eight concurrent return requests of 100000
- **THEN** exactly five are accepted and three fail with `INVALID_ARGUMENT`
- **AND** `ListOrderReturns` lists five returns totalling 500000

#### Scenario: An RMA refund larger than what remains refunds only the remainder

- **WHEN** a credited paid order of 500000 has an approved return of 300000, its seller refunds 400000 of the payment
  directly, and the seller then refunds the return
- **THEN** within the settle window the payment reads `REFUNDED` with a refunded amount of 500000
- **AND** the return's refund lists requested 300000 and applied 100000
- **AND** the seller's deductions for the order are -400000 and -100000

#### Scenario: An RMA refund after the payment was fully refunded records nothing to refund

- **WHEN** the seller of a credited paid order of 500000 refunds the payment in full directly, and then approves and
  refunds a return of 200000 on that order
- **THEN** within the settle window the payment stays `REFUNDED` with a refunded amount of 500000
- **AND** it lists the return's refund with requested 200000 and applied 0
- **AND** the seller has no `REFUND_DEDUCTION` for the return
- **AND** `order.events.payment-settlement.dlq` has no record for it

### Requirement: A ReturnRefunded that cannot be applied is parked

`team-order` never emits a `ReturnRefunded` for an order that was not paid online, so this is a defensive fallback.
A `ReturnRefunded` that is well-formed but whose order has no `PAID`, `PARTIALLY_REFUNDED` or `REFUNDED` payment SHALL
be parked on `order.events.payment-settlement.dlq` and write nothing. The same SHALL apply to a `ReturnRefunded` that lacks a return id or an order id,
or has a non-positive amount. Later records SHALL still be applied.

#### Scenario: A ReturnRefunded for an order without a paid payment is dead-lettered

- **WHEN** a well-formed `ReturnRefunded` record for an order that has no paid payment is produced to `order.events`,
  followed by a paid order
- **THEN** the record appears on `order.events.payment-settlement.dlq` and no refund is written
- **AND** the paid order's seller is credited once

### Requirement: An order's returns are listed to the order's parties

`ListOrderReturns` SHALL return all returns of an order, newest first, to three callers:

- the order's buyer;
- the order's seller;
- a principal with the `admin` scope.

Every other caller SHALL get `PERMISSION_DENIED`. An unknown order SHALL fail with `NOT_FOUND`. The gateway SHALL
route it without logic of its own.

#### Scenario: The seller lists the returns of their order

- **WHEN** a paid order has two returns and its seller calls `ListOrderReturns` through the gateway
- **THEN** both returns are listed newest first, each with its reason, refund amount and status

#### Scenario: Another buyer cannot list the returns of an order

- **WHEN** a buyer who did not place the order calls `ListOrderReturns` for it through the gateway
- **THEN** the call fails with `PERMISSION_DENIED`
