## MODIFIED Requirements

### Requirement: A paid order credits its seller exactly once

When an order becomes `Paid`, `team-payment` SHALL append exactly one `ORDER_SETTLEMENT` ledger entry with status
`COMPLETED` for the order's seller, referencing the order's payment transaction. Its amount SHALL be the amount of
that payment transaction plus the `platform_discount_amount` carried by the `OrderPaidEvent` (the platform funds the
platform voucher), and SHALL be the payment transaction amount alone when the event carries no price breakdown. The
credit SHALL be driven by `team-order`'s `OrderPaidEvent` on `order.events`, not by the payment RPC, so it appears
asynchronously after the payment succeeds. Paying an already paid order again, or settling one order concurrently
more than once, SHALL NOT add a second credit. `ProcessMockPayment` SHALL NOT write any ledger entry itself.

#### Scenario: Paying an order credits its seller with the paid amount

- **WHEN** a buyer pays a `Pending` order of 500000 with the mock payment, the order carries no platform voucher, and
  the order becomes `Paid`
- **THEN** within the settle window the seller's `ListLedgerEntries` through the gateway shows exactly one new
  `ORDER_SETTLEMENT` entry of +500000 with status `COMPLETED`
- **VERIFIED BY**: platform-e2e `payment/plp_settlement_credit.feature::Paying an order credits its seller with the paid amount`

#### Scenario: A platform voucher is credited to the seller on top of the paid amount

- **WHEN** a buyer pays a `Pending` order with items 300000, shipping 30000, shop voucher 0 and platform voucher
  10000, so the payment is 320000, and the order becomes `Paid`
- **THEN** the seller's ledger has exactly one `ORDER_SETTLEMENT` entry of +330000 referencing the payment
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A platform voucher is credited to the seller on top of the paid amount`

#### Scenario: An event without the price breakdown credits the paid amount only

- **WHEN** an `OrderPaidEvent` published before the breakdown existed (fields 9-12 absent) is consumed for a payment
  of 320000
- **THEN** the seller's ledger has exactly one `ORDER_SETTLEMENT` entry of +320000
- **AND** it has no `COMMISSION` entry for that payment
- **VERIFIED BY**: `team-payment/internal/settlement/breakdown_test.go` `TestConsumer_EventWithoutThePriceBreakdownCreditsThePaidAmountOnly`

#### Scenario: Paying an already paid order again does not credit twice

- **WHEN** the buyer calls the mock payment again for the same, already credited order
- **THEN** the call reports the order already paid, and after a later sentinel order of the same seller is credited
  the seller still has exactly one `ORDER_SETTLEMENT` entry for the first order's amount
- **VERIFIED BY**: platform-e2e `payment/plp_settlement_credit.feature::Paying an already paid order again does not credit twice`

#### Scenario: Concurrent settlements of one order credit the seller once

- **WHEN** the buyer sends eight concurrent successful mock payments for one `Pending` order
- **THEN** the order is `Paid` and, after a sentinel order of the same seller is credited, the seller has exactly one
  `ORDER_SETTLEMENT` entry for that order
- **VERIFIED BY**: platform-e2e `payment/plp_settlement_credit.feature::Concurrent settlements of one order credit the seller once`
