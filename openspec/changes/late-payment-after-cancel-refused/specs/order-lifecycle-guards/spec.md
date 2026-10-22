## MODIFIED Requirements

### Requirement: Only a still-pending order is marked paid by settlement

The `PaymentSettled` consumer SHALL move an order to `Paid` only from `Pending`, in the compare-and-set write that also
records `paid_at`. A settled event for an order in any other status SHALL be acknowledged without changing the order and
logged with the order id and its current status. The consumer SHALL commit the order's voucher hold only for an order
that reached `Paid` (`paid_at` set), so a cancelled order's voucher is never redeemed by a late payment. Redelivery SHALL
remain a no-op. A payment attempted after the order is `Cancelled` SHALL be refused by team-payment (see
`unpaid-order-payment`), so it never reaches this consumer.

#### Scenario: A late payment after cancel is ignored

- **WHEN** a buyer starts an online payment for a `Pending` order, cancels the order, and then pays it
- **THEN** the payment call fails with `failed_precondition`, the order read through the gateway stays `Cancelled` and
  its listing's stock is restored once

#### Scenario: Payment racing cancel ends cancelled with stock restored once

- **WHEN** a buyer's online payment succeeds while the buyer cancels the same `Pending` order at the same time
- **THEN** the order ends `Cancelled` and its listing's stock is restored once

#### Scenario: A late payment of a cancelled voucher order leaves the voucher unused

- **WHEN** a buyer checks out with a voucher whose quota is 1, cancels the order, and then pays it
- **THEN** the payment call fails with `failed_precondition`, the order stays `Cancelled` and another buyer can still
  check out with that voucher
