@analytics @batch @integration
Feature: Paid orders become a user feature
  `python -m featurestore materialize` computes `paid_orders_30d` in `user_activity@v2` from the buyer
  column of `order_facts`. It runs here as the real job image on the stack network, writing to Redis DB 3
  (not DB 2, so the featurestore-materialization pointers are left alone). The first scenario uses the
  real analytics export; the others feed the same image synthetic order lines so the 30-day window,
  the order count and the missing buyer can be set exactly.
  (order-facts-buyer / feature-materialization)

  Scenario: A paid order becomes a user feature
    When a new buyer pays one order, the export cycle completes, and the materialisation job runs
    Then the online paid_orders_30d of that buyer under user_activity@v2 is 1, and that buyer has a row although they posted no tracking event

  Scenario: Only orders in the 30 days before AS_OF count
    When a user has paid orders 31 days, 29 days and 1 day before AS_OF and one after AS_OF, and the job runs
    Then the user's paid_orders_30d is 2

  Scenario: A multi-line order counts once
    When a user has one paid order with three lines and one paid order with one line within the window, and the job runs
    Then the user's paid_orders_30d is 2

  Scenario: An order without a buyer counts for nobody
    When the order facts hold a paid order line with no buyer_id, and the job runs
    Then no user_activity@v2 row has a paid_orders_30d that includes that order

  Scenario: A tampered order count fails the parity check
    When after a run, one buyer's user_activity@v2 online paid_orders_30d is overwritten with a different value, and python -m featurestore parity runs
    Then the command exits non-zero and its output names that buyer and paid_orders_30d
