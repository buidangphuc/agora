# order-facts Specification

## Purpose
TBD - created by archiving change add-order-warehouse-facts. Update Purpose after archive.

## Requirements

### Requirement: Transactional Outbox for Order Settlement
When an order transitions to `PAID`, `team-order` MUST atomically persist an outbox event containing the order's line items and monetary details in the same database transaction.

#### Scenario: Order settlement commits an outbox event atomically
- Given an order in `PENDING` status with line items
- When the order transitions to `PAID`
- Then an outbox event is stored in `order_outbox_events` with status `pending` and the exact line item quantities and prices

### Requirement: Order Events Publishing to Kafka
A background relayer in `team-order` MUST read pending outbox events, produce them to the `order.events` Kafka topic wrapped in `platform.events.v1.EventEnvelope`, and update outbox status to `published`.

#### Scenario: Relayer publishes pending order events to Kafka
- Given pending records in `order_outbox_events`
- When the outbox relayer runs a sweep
- Then events are published to `order.events` partitioned by `order_id`
- And the outbox records are marked `published` with a `published_at` timestamp

### Requirement: Analytics Ingestion into order_facts
`team-analytics` MUST consume `order.events`, unpack line items, and append them into the columnar `order_facts` warehouse table with deduplication support.

#### Scenario: Consuming an order paid event appends rows to order_facts
- Given an `OrderPaidEvent` on Kafka topic `order.events` with multiple line items
- When `team-analytics` processes the message
- Then one row per line item is inserted into `order_facts` with matching `seller_id`, `listing_id`, `quantity`, and `unit_price`

### Requirement: Real Revenue Aggregation from order_facts
`AnalyticsQueryService.GetRevenueBreakdown` and `GetSellerFunnel` MUST compute revenue and unit metrics directly from `order_facts` without parsing unstructured JSON properties.

#### Scenario: GetRevenueBreakdown aggregates real revenue from order_facts
- Given multiple order lines in `order_facts` for a seller across several dates
- When `GetRevenueBreakdown` is queried for that seller
- Then top SKUs report accurate `revenue` and `units_sold` calculated as `SUM(quantity * unit_price)` and `SUM(quantity)`
