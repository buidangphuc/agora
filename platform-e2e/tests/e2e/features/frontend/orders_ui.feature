@buyer @order
Feature: Buyer orders UI - list, detail, timeline and return flow
  The rebuilt /account/orders and /account/orders/[id] screens (ui-phase-orders):
  status tabs and pagination held in the URL, the saga failure checkpoint, the
  return Modal with validation, the 403 page for another buyer's order and the
  375px layout. Needs the full local stack (gateway, order, payment, listing).

  @needsBuyer @needsListing
  Scenario: Selecting a status tab updates the URL and the list
    Given I am logged in as a buyer via API
    And the buyer has a pending, a delivered and a cancelled order
    When I navigate to the "account orders" page
    And I select the "Đã giao" order tab
    Then the orders URL is "/account/orders?status=completed"
    And every listed order has the status "Đã hoàn thành"
    And the orders list shows 1 order

  @needsBuyer @needsListing
  Scenario: The list state survives reload and back navigation
    Given I am logged in as a buyer via API
    And the buyer has 11 orders
    When I open the orders list with the query "status=pending&page=2"
    And I reload the page
    Then the orders URL is "/account/orders?status=pending&page=2"
    And the "Chờ xử lý" order tab is the current tab
    And the orders list shows 1 order
    When the d2 buyer navigates away and comes back with the browser back button
    Then the orders URL is "/account/orders?status=pending&page=2"
    And the "Chờ xử lý" order tab is the current tab
    And the orders list shows 1 order

  @needsBuyer @needsListing
  Scenario: Pagination links page through the filtered list
    Given I am logged in as a buyer via API
    And the buyer has 23 orders
    When I navigate to the "account orders" page
    Then the orders list shows 10 orders
    And the pagination has 3 pages
    When I follow the pagination link "3"
    Then the orders URL is "/account/orders?page=3"
    And the orders list shows 3 orders

  @needsBuyer @needsListing
  Scenario: An empty tab offers a way back
    Given I am logged in as a buyer via API
    And the buyer has 1 orders
    When I open the orders list with the query "status=cancelled"
    Then the empty tab message is shown with a link to all orders
    When I follow the link to all orders
    Then the orders URL is "/account/orders"
    And the orders list shows 1 order

  @needsBuyer @needsListing
  Scenario: A failed saga step is surfaced as the failure checkpoint
    Given I am logged in as a buyer via API
    And the buyer has an order whose payment failed
    When I open the order detail page
    Then the timeline shows the failure checkpoint
    And the failure alert offers "Mua lại"

  @needsBuyer @needsListing
  Scenario: The return Modal validates and then submits
    Given I am logged in as a buyer via API
    And the buyer has a delivered order
    When I open the order detail page
    And I open the return Modal
    And I submit the return form without a reason
    Then the return form shows the reason error and stays open
    When I submit the return form with reason "defective"
    Then the return section shows the request as pending

  @needsBuyer @needsListing
  Scenario: An order of another user shows 403
    Given I am logged in as a buyer via API
    And an order belongs to a different buyer
    When I open the order detail page
    Then the 403 page is shown with a link back to my orders
    And no item of that order is rendered
    And no recipient, item or amount of that order appears in the page

  @needsBuyer @needsListing @wap
  Scenario: The order screens fit a 375px viewport
    Given I am logged in as a buyer via API
    And the buyer has a pending, a delivered and a cancelled order
    And the viewport is 375 by 812
    When I navigate to the "account orders" page
    Then the page has no horizontal scroll
    When I open the order detail from the list
    Then the page has no horizontal scroll

  @needsBuyer @needsListing
  Scenario: Tracking events still fire after reorder
    Given I am logged in as a buyer via API
    And the buyer has 1 orders
    When I navigate to the "account orders" page
    And I click "Mua lại" on the first order
    Then the buyer lands on the cart
    And the analytics data layer is initialised
