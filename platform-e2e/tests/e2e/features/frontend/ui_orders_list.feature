@buyer @order
Feature: Buyer order list - URL state, real data, badges and empty states
  OpenSpec change ui-phase-orders, /account/orders. Orders are seeded through the gateway
  as the logged-in buyer and the order's seller. (The status tab, reload, pagination and
  empty-tab scenarios live in frontend/orders_ui.feature.)

  Scenario: An invalid query falls back safely
    Given a d2 buyer has 11 orders from one shop
    When the d2 buyer opens the orders list with the query "status=bogus&page=99"
    Then the all tab is active and the last page is shown without an error

  Scenario: A guest is redirected
    Given a d2 buyer has no orders
    When the d2 buyer opens the orders list as a guest
    Then the browser lands on the login page

  Scenario: Order row shows the real shop name
    Given a d2 buyer has an order from a shop named "Cửa hàng Hoa Mai"
    When the d2 buyer opens the orders list
    Then the order row shows "Cửa hàng Hoa Mai" and not "Shop #"

  Scenario: Empty shop name falls back
    Given a d2 buyer has an order from a shop named "-"
    When the d2 buyer opens the orders list
    Then the order row shows "Shop #" followed by the first 6 characters of the seller id

  Scenario: Statuses map to tones
    Given a d2 buyer has a pending, a shipped, a completed and a cancelled order
    When the d2 buyer opens the orders list
    Then the pending, shipped, completed and cancelled orders render warning, info, success and neutral badges

  Scenario: A buyer with no orders sees Empty with a shopping action
    Given a d2 buyer has no orders
    When the d2 buyer opens the orders list
    Then the empty state offers a shopping link and every tab count reads 0

  Scenario: Loading does not shift layout
    Given a d2 buyer has 3 orders from one shop
    When the d2 buyer opens the orders list from the cart while the list data is slow
    Then a tab bar and three order card skeletons show first and the content then replaces them without a layout shift
