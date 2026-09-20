@analytics @order @integration
Feature: Order facts outbox emission and warehouse ingestion
  As the platform, when orders transition to PAID status, line items are atomically
  recorded in the transactional outbox, relayed to Kafka order.events, consumed into
  the order_facts warehouse table, and aggregated by AnalyticsQueryService.

  Scenario: Order settlement commits an outbox event atomically
    Given an order in "PENDING" status with line items
    When the order transitions to "PAID"
    Then an outbox event is stored in "order_outbox_events" with status "pending" and the exact line item quantities and prices

  Scenario: Relayer publishes pending order events to Kafka
    Given pending records in "order_outbox_events"
    When the outbox relayer runs a sweep
    Then events are published to "order.events" partitioned by "order_id"
    And the outbox records are marked "published" with a "published_at" timestamp

  Scenario: Consuming an order paid event appends rows to order_facts
    Given an "OrderPaidEvent" on Kafka topic "order.events" with multiple line items
    When "team-analytics" processes the message
    Then one row per line item is inserted into "order_facts" with matching "seller_id", "listing_id", "quantity", and "unit_price"

  Scenario: GetRevenueBreakdown aggregates real revenue from order_facts
    Given multiple order lines in "order_facts" for a seller across several dates
    When "GetRevenueBreakdown" is queried for that seller
    Then top SKUs report accurate "revenue" and "units_sold" calculated as "SUM(quantity * unit_price)" and "SUM(quantity)"

  Scenario: GetDemandForecast serves probabilistic daily demand and restock points
    Given a seller SKU with historical order facts
    When "GetDemandForecast" is queried for that seller and listing
    Then a multi-day forecast with "p10", "p50", "p90" quantiles and suggested reorder point is returned

