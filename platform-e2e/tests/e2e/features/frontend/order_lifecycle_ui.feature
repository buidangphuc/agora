@order @lifecycle_ui
Feature: Order lifecycle in the buyer and seller UI
  OpenSpec change frontend-order-lifecycle (initiative q10-order-lifecycle). The UI shows the
  server's lifecycle decisions: cancel with a reason code, pay again before the deadline,
  confirm receipt, return inside the window, the seller's ship-by deadline and the lifecycle
  notification links. Orders are seeded through the gateway by real, distinct users; timer
  scenarios run with compose/order-inventory.override.yaml (payment deadline 45s, reminder
  30s before, auto-complete 12s, return window 20s, ship-by 60s, sweep 2s) and are polled.

  Scenario: Order lifecycle: the buyer cancels a paid order with a reason
    Given a lifecycle buyer has a paid online order
    When the lifecycle buyer opens the order detail
    And the lifecycle buyer cancels it choosing the reason "Sản phẩm bị lỗi / hư hỏng"
    Then the lifecycle order detail shows the status "Đã hủy"
    And the lifecycle order reads back cancel reason "CANCEL_REASON_DAMAGED"

  Scenario: Order lifecycle: a failed payment can be retried from the order list
    Given a lifecycle buyer has an online order whose payment failed
    When the lifecycle buyer opens the order list
    And the lifecycle buyer presses "Thanh toán lại" on the order
    Then the lifecycle pay page offers the payment simulator
    When the lifecycle buyer pays successfully on the pay page
    And the lifecycle buyer opens the order list
    Then the lifecycle order list shows the status "Đã thanh toán"
    And the lifecycle order reads back status "ORDER_STATUS_PAID"

  Scenario: Order lifecycle: the pay page opens for an order that has no transaction
    Given a lifecycle buyer has an online order with no payment transaction
    When the lifecycle buyer opens the pay page of the order
    Then the lifecycle pay page answers 200 and offers the payment simulator
    And the lifecycle payment of the order reads back status "PAYMENT_STATUS_PENDING"

  @lifecycle_timers @slow
  Scenario: Order lifecycle: an unpaid order expires and the pay page says so
    Given a lifecycle buyer has an online order awaiting payment
    And the lifecycle pay page offers the payment simulator before the deadline
    When the lifecycle payment deadline passes without payment
    And the lifecycle buyer opens the order list
    Then the lifecycle order list shows the status "Đã hủy"
    When the lifecycle buyer opens the pay page of the order
    Then the lifecycle pay page shows "Đơn hàng đã hết hạn thanh toán" and no payment buttons
    And the lifecycle order reads back cancel reason "CANCEL_REASON_PAYMENT_EXPIRED"

  Scenario: Order lifecycle: the buyer confirms receipt and the review prompt opens
    Given a lifecycle buyer has a paid online order
    And the lifecycle seller ships the order
    When the lifecycle buyer opens the order detail
    And the lifecycle buyer confirms receipt of the order
    Then the lifecycle order detail shows the status "Đã hoàn thành"
    And the lifecycle order detail offers the "Đánh giá" button
    And the lifecycle order reads back status "ORDER_STATUS_COMPLETED" with a completion time

  @lifecycle_timers @slow
  Scenario: Order lifecycle: the system completes a shipped order when the buyer does not
    Given a lifecycle buyer has a paid online order
    And the lifecycle seller ships the order
    When the lifecycle buyer opens the order detail
    Then the lifecycle order detail offers "Đã nhận được hàng"
    When the lifecycle auto-complete time passes
    And the lifecycle buyer opens the order detail
    Then the lifecycle order detail shows the status "Đã hoàn thành"
    And the lifecycle order detail offers the "Đánh giá" button
    And the lifecycle order detail no longer offers "Đã nhận được hàng"

  Scenario: Order lifecycle: a return is accepted on a shipped order
    Given a lifecycle buyer has a paid online order
    And the lifecycle seller ships the order
    When the lifecycle buyer opens the order detail
    And the lifecycle buyer submits a return with the reason "defective"
    Then the lifecycle returns tab shows a "Chờ duyệt" badge
    And the lifecycle order has exactly one PENDING return

  @lifecycle_timers @slow
  Scenario: Order lifecycle: after the return window no return is offered
    Given a lifecycle buyer has a paid online order
    And the lifecycle seller ships the order
    And the lifecycle order completed by itself
    When the lifecycle buyer opens the order detail
    Then the lifecycle order detail offers "Yêu cầu trả hàng" and "Có thể trả hàng đến"
    When the lifecycle return window passes
    And the lifecycle buyer opens the order detail
    Then the lifecycle order detail shows "Đã hết thời hạn trả hàng" and no "Yêu cầu trả hàng"
    And a lifecycle return request through the gateway is refused with "failed_precondition"

  @lifecycle_timers @slow
  Scenario: Order lifecycle: the seller dashboard flags an overdue order
    Given a lifecycle seller is signed in with a paid online order left unshipped
    When the lifecycle seller opens the dashboard
    Then the lifecycle dashboard shows "Cần gửi" without "Quá hạn"
    When the lifecycle ship-by time passes
    And the lifecycle seller opens the dashboard
    Then the lifecycle dashboard shows "Cần gửi" with "Quá hạn: 1"
    When the lifecycle seller opens the seller order detail
    Then the lifecycle seller order detail shows "Hạn gửi" and the "Quá hạn" tag

  Scenario: Order lifecycle: the new-order notification opens the seller order
    Given a lifecycle seller is signed in with a new order placed by a buyer
    When the lifecycle seller opens the notifications page
    And the lifecycle seller follows the "Đơn hàng mới" notification
    Then the lifecycle seller lands on the seller order page of that order with status 200

  Scenario: Order lifecycle: another seller cannot open the new-order link
    Given a lifecycle seller is signed in with a new order placed by a buyer
    And the lifecycle owning seller can open the seller order page
    When a different lifecycle seller opens the seller order page of that order
    Then the lifecycle response is the not-found page with status 404 and no order data

  @lifecycle_timers @slow
  Scenario: Order lifecycle: the payment reminder opens the pay page
    Given a lifecycle buyer has an online order awaiting payment
    When the lifecycle buyer opens the notifications page
    Then the lifecycle buyer sees exactly one payment reminder
    When the lifecycle buyer follows the payment reminder
    Then the lifecycle pay page offers the payment simulator
