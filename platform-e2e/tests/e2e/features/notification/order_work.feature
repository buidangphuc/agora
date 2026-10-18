@notification @order
Feature: Order lifecycle events reach the right user's notification feed
  team-notification turns the team-order lifecycle events into in-app notifications: a seller is
  told of a new order (COD at placement, online at payment), of a requested return and of low
  stock; a buyer is told of the payment reminder, the expiry cancel and completion. Each
  notification reaches only its recipient, exactly once. (notification-order-work, initiative
  q10-order-lifecycle; e2e overlay: payment deadline 45s, reminder 30s before, sweep every 2s)

  Scenario: Order work: A COD order reaches the seller feed end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a COD order for 1 of "L"
    Then "sa" has exactly one "Đơn hàng mới" notification linking to "/seller/orders/{order_id}"
    And "b1" has no notification linking to "/seller/orders/{order_id}"

  Scenario: Order work: A paid online order reaches the seller feed once end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 1 of "L"
    Then "sa" has no "Đơn hàng mới" notification linking to "/seller/orders/{order_id}"
    When "b1" pays the order
    Then "sa" has exactly one "Đơn hàng mới" notification linking to "/seller/orders/{order_id}"

  Scenario: Order work: An unpaid order reminds and then expires its buyer end to end
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    When "b1" places a MOCK_MOMO order for 1 of "L"
    Then "b1" has exactly one "Đơn hàng sắp hết hạn thanh toán" notification linking to "/checkout/pay/{order_id}"
    And "b1" has no notification titled "Đơn hàng đã bị hủy do quá hạn thanh toán" for the order
    When the payment deadline passes without payment
    Then "b1" has exactly one "Đơn hàng đã bị hủy do quá hạn thanh toán" notification linking to "/account/orders/{order_id}"
    And "sa" has no notification linking to "/checkout/pay/{order_id}"

  Scenario: Order work: A return request reaches the seller feed end to end
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And "b1" has a shipped COD order for "L"
    When "b1" requests a return of 100000 on the order
    Then "sa" has exactly one "Yêu cầu trả hàng mới" notification linking to "/seller/orders/{order_id}?tab=returns"
    And "b1" has no notification titled "Yêu cầu trả hàng mới" for the order

  Scenario: Order work: A completed order reaches the buyer feed end to end
    Given a seller "sa" with a listing "L" priced 600000
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has a shipped COD order for "L"
    When "b1" confirms receipt of the order
    Then "b1" has exactly one "Đơn hàng đã hoàn tất" notification linking to "/account/orders/{order_id}"
    And "b2" has no notification linking to "/account/orders/{order_id}"
    And "sa" has no notification titled "Đơn hàng đã hoàn tất" for the order

  Scenario: Order work: Selling stock down notifies the seller once end to end
    Given a seller "sa" who owns a listing "L" with stock 6
    And a buyer "b1"
    When "b1" places a COD order for 2 of "L"
    Then "sa" has exactly one "Sắp hết hàng" notification linking to the edit page of "L"
    When "b1" places a COD order for 1 of "L"
    Then the stock of "L" is 3
    And "sa" still has exactly one "Sắp hết hàng" notification linking to the edit page of "L"
