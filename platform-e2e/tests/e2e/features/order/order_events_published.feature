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

  Scenario: Paid order's event matches GetOrder
    Given a shop "A" selling a listing priced 300000
    And a shop voucher "VA" of shop "A" worth 20000 off
    And a platform voucher "PV" worth 30000 off with min spend 0
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    And "b1" applies shop voucher "VA" to shop "A"
    And "b1" applies platform voucher "PV"
    When "b1" places the order with the same selection
    And "b1" pays the first placed order online
    Then the single OrderPaidEvent on the "order.events" topic for the first placed order of "b1" matches its GetOrder price breakdown
    And that breakdown has a positive shipping fee, a positive platform discount and a positive shop discount

  Scenario: Event for an order without a voucher reports zero discounts
    Given a shop "A" selling a listing priced 300000
    And a buyer "b1" in "Đà Nẵng" with 1 of each shop's listing in the cart
    When "b1" places the order with the same selection
    And "b1" pays the first placed order online
    Then the single OrderPaidEvent on the "order.events" topic for the first placed order of "b1" matches its GetOrder price breakdown
    And that breakdown reports shop_discount_amount 0 and platform_discount_amount 0 with presence true
