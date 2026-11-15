@order
Feature: ListSellerOrders pages, filters, searches and counts the seller's own orders
  Black box through the gateway. Orders are seeded by real checkouts and status updates.
  (order-seller-list-paging, initiative seller-centre)

  Scenario: Pages cover the seller's orders exactly once
    Given a seller "A" owning 250 orders
    And the request has page_size 100
    When "A" follows next_cursor until it is empty
    Then the pages hold 100, 100 and 50 orders
    And no order id repeats across the pages
    And created_at never increases across the pages
    And every page reports page.total 250

  Scenario: Default page size is 20
    Given a seller "A" owning 30 orders
    And the request has an empty page
    When "A" lists seller orders
    Then the call succeeds
    And the response holds 20 orders
    And the next_cursor is non-empty

  Scenario: Seller orders: Page size above the maximum is clamped
    Given a seller "A" owning 150 orders
    And the request has page_size 1000
    When "A" lists seller orders
    Then the call succeeds
    And the response holds 100 orders
    And the next_cursor is non-empty

  Scenario: A negative page size is rejected
    Given a seller "A" owning 2 orders
    And the request has page_size -1
    When "A" lists seller orders
    Then the call fails with invalid_argument
    And no order is returned

  Scenario: An unknown cursor is rejected
    Given a seller "A" owning 2 orders
    And the request has an empty page
    And the request has cursor "not-a-cursor"
    When "A" lists seller orders
    Then the call fails with invalid_argument
    And no order is returned

  Scenario: A cursor never reaches another seller's orders
    Given a seller "A" owning 30 orders
    And a seller "B" owning 2 orders
    And "A" holds a next_cursor from a first page of 10
    And the request has an empty page
    And the request uses the cursor of "A"
    When "B" lists seller orders
    Then every returned order belongs to "B"

  Scenario: A seller with no orders gets an empty page
    Given a seller "A" owning no order
    And the request has an empty page
    When "A" lists seller orders
    Then the call succeeds
    And the response holds 0 orders
    And page.total is 0
    And the next_cursor is empty
    And status_counts is PENDING 0, PAID 0, SHIPPED 0, COMPLETED 0, CANCELLED 0

  Scenario: An unauthenticated caller is denied
    Given a seller "A" owning 1 orders
    And the request has an empty page
    When an unauthenticated caller lists seller orders
    Then the call fails with unauthenticated
    And no order is returned

  @slow
  Scenario: Paged latency at 5,000 orders
    Given a seller "A" owning 5000 orders of which 200 are PAID
    And the request has statuses "PAID"
    And the request has page_size 20
    When "A" lists seller orders 50 times
    Then the 95th percentile of the call durations is at most 150 ms

  Scenario: One status tab
    Given a seller "A" owning 3 PAID, 2 SHIPPED and 1 CANCELLED orders
    And the request has statuses "PAID"
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds exactly the PAID orders of "A"
    And page.total is 3

  Scenario: Seller orders: Several statuses are OR-ed
    Given a seller "A" owning 3 PAID, 2 SHIPPED and 1 CANCELLED orders
    And the request has statuses "SHIPPED,CANCELLED"
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds exactly the SHIPPED and CANCELLED orders of "A"
    And page.total is 3

  Scenario: statuses wins over status_filter
    Given a seller "A" owning 3 PAID, 2 SHIPPED and 1 CANCELLED orders
    And the request has status_filter "CANCELLED"
    And the request has statuses "PAID"
    When "A" lists seller orders
    Then the response holds exactly the PAID orders of "A"

  Scenario: Seller orders: An unspecified status is rejected
    Given a seller "A" owning 2 orders
    And the request has statuses "UNSPECIFIED"
    When "A" lists seller orders
    Then the call fails with invalid_argument
    And no order is returned

  Scenario: Item title substring
    Given a seller "A" who has a PENDING order titled "Áo thun Đỏ"
    And "A" has a PENDING order titled "Quần jean"
    And the request has query "áo thun"
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds only the order titled "Áo thun Đỏ"
    And page.total is 1

  Scenario: Order id prefix
    Given a seller "A" owning 5 orders
    And the request has the first 8 characters of an order id of "A" as query
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds that order and every own order sharing the prefix

  Scenario: Seller orders: Query and status combine
    Given a seller "A" who has a PAID order titled "Áo thun xanh"
    And "A" has a SHIPPED order titled "Áo thun vàng"
    And the request has query "áo"
    And the request has statuses "SHIPPED"
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds only the order titled "Áo thun vàng"

  Scenario: Seller orders: LIKE wildcards match literally
    Given a seller "A" who has a PENDING order titled "Giảm 50% áo"
    And "A" has a PENDING order titled "Giảm 500 áo"
    And the request has query "50%"
    And the request has an empty page
    When "A" lists seller orders
    Then the response holds only the order titled "Giảm 50% áo"

  Scenario: Seller orders: A query of exactly 100 runes is accepted, 101 is rejected
    Given a seller "A" owning 2 orders
    And the request has an empty page
    And the request has a query of 100 runes
    When "A" lists seller orders
    Then the call succeeds
    And the response holds 0 orders
    When the request has a query of 101 runes
    And "A" lists seller orders
    Then the call fails with invalid_argument

  Scenario: Seller orders: A blank query is ignored
    Given a seller "A" owning 4 orders
    And the request has an empty page
    And the request has query "   "
    When "A" lists seller orders
    Then the call succeeds
    And the response equals the response of the same request with an empty query

  Scenario: Seller orders: No match returns an empty page
    Given a seller "A" owning 3 orders
    And the request has query "zzzz-no-such-title"
    And the request has an empty page
    When "A" lists seller orders
    Then the call succeeds
    And the response holds 0 orders
    And page.total is 0
    And the next_cursor is empty

  Scenario: Another seller's orders are never returned
    Given a seller "A" who has a PENDING order titled "Áo khoác riêng của A"
    And a seller "B" owning no order
    And the request has query "Áo khoác riêng của A"
    And the request has an empty page
    When "B" lists seller orders
    Then the call succeeds
    And the response holds 0 orders
    And page.total is 0
    When the request has the id of the order of "A" as query
    And "B" lists seller orders
    Then the response holds 0 orders
    And page.total is 0

  @destructive
  Scenario: Seller orders: The query text is not logged
    Given a seller "A" owning 2 orders
    And the request has query "secret-needle-title"
    And the request has an empty page
    When "A" lists seller orders
    Then the call succeeds
    And no team-order-svc log line since the call contains "secret-needle-title"

  Scenario: Counts ignore the selected tab
    Given a seller "A" owning 3 PAID, 2 SHIPPED and 1 CANCELLED orders
    And the request has statuses "PAID"
    And the request has an empty page
    When "A" lists seller orders
    Then status_counts is PENDING 0, PAID 3, SHIPPED 2, COMPLETED 0, CANCELLED 1

  Scenario: Counts never include another seller's orders
    Given a seller "A" owning 3 PAID orders
    And a seller "B" owning 2 PAID orders
    And the request has an empty page
    When "B" lists seller orders
    Then status_counts is PENDING 0, PAID 2, SHIPPED 0, COMPLETED 0, CANCELLED 0

  Scenario: A rejected request returns no counts
    Given a seller "A" owning 2 orders
    And the request has statuses "UNSPECIFIED"
    When "A" lists seller orders
    Then the call fails with invalid_argument
    And the response carries no status_counts

  Scenario: Counts follow the query
    Given a seller "A" who has a PAID order titled "Áo thun xanh"
    And "A" has a SHIPPED order titled "Áo thun vàng"
    And "A" has a CANCELLED order titled "Quần jean"
    And the request has query "áo thun"
    And the request has an empty page
    When "A" lists seller orders
    Then status_counts is PENDING 0, PAID 1, SHIPPED 1, COMPLETED 0, CANCELLED 0

  Scenario: Legacy call returns the full list
    Given a seller "A" owning 250 orders of which 120 are PAID
    And the request has status_filter "PAID"
    When "A" lists seller orders
    Then the response holds all 120 PAID orders of "A" newest first
    And the next_cursor is empty
    And page.total is 120

  Scenario: Legacy call without any field
    Given a seller "A" owning 6 orders
    When "A" lists seller orders
    Then the response holds all 6 orders of "A" newest first
