@analytics @order @integration
Feature: Order facts record the buyer
  `OrderPaidEvent.buyer_id` is stored on every `order_facts` row and exported in
  `order_facts.parquet`, so per-user order features can be computed from the warehouse.
  (order-facts-buyer / order-facts)

  The scenario pays a real order through the gateway and waits for the next export cycle
  (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s locally; override the wait with OFB_EXPORT_WAIT_S).

  Scenario: A paid order's lines carry the buyer
    When a buyer pays an order with two lines of different listings and the next export cycle completes
    Then order_facts.parquet on the analytics volume has two rows for that order, each with buyer_id equal to that buyer's user id
