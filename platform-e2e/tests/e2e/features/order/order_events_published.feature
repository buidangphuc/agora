@buyer @needsBuyer @needsListing @needsOrder @order @integration
Feature: A paid order is published to order.events
  As the platform, when an order transitions to PAID, team-order writes an outbox row in
  the same transaction and its relayer publishes an OrderPaidEvent (with line items),
  wrapped in an EventEnvelope and keyed by order_id, to the order.events Kafka topic
  that team-analytics consumes into order_facts (ADR-0013).

  Scenario: Paying an order publishes an OrderPaidEvent with its line items
    Given a buyer has a pending order
    When the buyer completes the demo payment
    Then the order status becomes PAID
    And exactly one OrderPaidEvent envelope for the order is published to the "order.events" topic
    And its payload carries the order id as the key and the order's line items
