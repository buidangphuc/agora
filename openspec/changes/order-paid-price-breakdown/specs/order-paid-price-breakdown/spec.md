# order-paid-price-breakdown Specification

## ADDED Requirements

### Requirement: OrderPaidEvent carries the order's price breakdown

When an order moves to PAID, team-order SHALL build the `OrderPaidEvent` with `items_subtotal` (field 9) = the order's items subtotal, `shipping_fee` (10) = its shipping fee, `platform_discount_amount` (12) = its platform voucher share, and `shop_discount_amount` (11) = `discount_amount - platform_discount_amount` (the shop-funded voucher share), all integer VND, read from the stored order and never from request input. Zero values SHALL be set explicitly so field presence is true for every published breakdown. Fields 1-8 and the event id SHALL be unchanged.

#### Scenario: Shop voucher only
- **WHEN** an order with items_subtotal 300000, shipping_fee 30000, discount_amount 20000, platform_discount_amount 0 and total_amount 310000 moves to PAID
- **THEN** the outbox event decodes with items_subtotal 300000, shipping_fee 30000, shop_discount_amount 20000, platform_discount_amount 0 and total_amount 310000
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventCarriesBreakdownShopVoucher`

#### Scenario: Shop and platform voucher split
- **WHEN** an order with items_subtotal 300000, shipping_fee 30000, discount_amount 50000 of which platform_discount_amount 30000, and total_amount 280000 moves to PAID
- **THEN** the event decodes with shop_discount_amount 20000 and platform_discount_amount 30000
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventSplitsShopAndPlatformDiscount`

#### Scenario: No discount sets explicit zeros
- **WHEN** an order with items_subtotal 300000, shipping_fee 30000, discount_amount 0 and total_amount 330000 moves to PAID
- **THEN** the event reports presence true for shop_discount_amount and platform_discount_amount, both equal to 0
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventZeroDiscountIsPresent`

#### Scenario: Existing fields and event id are unchanged
- **WHEN** an order is moved to PAID and its event is built
- **THEN** fields 1-8 equal the values the previous build produced for the same order and event_id equals `OrderPaidEventID(order.ID)`
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventLegacyFieldsUnchanged`

#### Scenario: Breakdown is committed in the PAID transaction
- **WHEN** a PENDING order with a platform voucher share is moved to PAID against a real database
- **THEN** exactly one `order_outbox_events` row exists whose payload carries the breakdown matching the stored order
- **VERIFIED BY**: team-order `internal/store` `TestPaidTransitionOutboxCarriesBreakdown`

#### Scenario: A failing enqueue rolls the PAID transition back
- **WHEN** the outbox insert fails while an order with a breakdown moves to PAID
- **THEN** the move returns an error, the order still reads PENDING and zero outbox rows exist for it
- **VERIFIED BY**: team-order `internal/store` `TestPaidTransitionRollsBackOnEnqueueFailure`

### Requirement: An incoherent breakdown degrades to the old event shape and never blocks payment

team-order SHALL attach fields 9-12 only when `items_subtotal + shipping_fee - shop_discount_amount - platform_discount_amount == total_amount` and both discount shares are >= 0. Otherwise it SHALL publish the event with fields 9-12 absent, SHALL still complete the PAID transition, and SHALL increment the counter `order_paid_breakdown_skipped_total` and log one error line without raw ids.

#### Scenario: Platform share larger than the discount is skipped
- **WHEN** an order with discount_amount 10000 and platform_discount_amount 30000 moves to PAID
- **THEN** the transition succeeds, the event has presence false on fields 9-12 and the counter increases by 1
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventSkipsNegativeShopShare`

#### Scenario: Amounts that do not sum to the total are skipped
- **WHEN** an order whose items_subtotal + shipping_fee - discount_amount differs from total_amount moves to PAID
- **THEN** the transition succeeds, the event has presence false on fields 9-12 and the counter increases by 1
- **VERIFIED BY**: team-order `internal/events` `TestPaidEventSkipsIncoherentTotal`

#### Scenario: Outbox rows enqueued before deploy are not rewritten
- **WHEN** a pending `order_outbox_events` row written by the previous build is relayed after the new build starts
- **THEN** the published payload is byte-identical to the stored payload and has presence false on fields 9-12
- **VERIFIED BY**: team-order `internal/store` `TestPendingLegacyPaidRowRelayedUnchanged`

#### Scenario: Re-emitted PAID fact collapses to one row
- **WHEN** the PAID transition is replayed for an order whose event already exists
- **THEN** exactly one `order_outbox_events` row exists for the order and its payload is the first one written
- **VERIFIED BY**: team-order `internal/store` `TestPaidReplayKeepsSingleBreakdownRow`

### Requirement: The published event on the broker carries the breakdown end to end

On the real broker, the `OrderPaidEvent` for an order paid online SHALL carry the breakdown fields equal to the amounts the order reports through `GetOrder`.

#### Scenario: Paid order's event matches GetOrder
- **WHEN** a buyer completes the demo payment for a pending order with a shipping fee and a voucher
- **THEN** the single OrderPaidEvent on the "order.events" topic has items_subtotal, shipping_fee, shop_discount_amount and platform_discount_amount equal to the order's items_subtotal, shipping_fee, discount_amount minus platform_discount_amount and platform_discount_amount
- **VERIFIED BY**: platform-e2e `order/order_events_published.feature::Paid order's event matches GetOrder`

#### Scenario: Event for an order without a voucher reports zero discounts
- **WHEN** a buyer completes the demo payment for a pending order without any voucher
- **THEN** the OrderPaidEvent on the "order.events" topic has shop_discount_amount 0 and platform_discount_amount 0 with presence true
- **VERIFIED BY**: platform-e2e `order/order_events_published.feature::Event for an order without a voucher reports zero discounts`
