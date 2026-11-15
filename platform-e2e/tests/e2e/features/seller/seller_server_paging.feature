@seller
Feature: The seller app reads one bounded page at a time
  /seller/orders and /seller fetch only the displayed page, filter, search and count in the owning
  services, the ads and bundle pickers search by typing, and a failing source degrades only its own
  block. Orders and listings are seeded through the gateway; outages stop the real service.
  (seller-server-paging-ui, initiative seller-centre)

  Scenario: First page of a large shop
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the orders page
    Then the orders table shows 20 rows
    And the "Tất cả" badge shows 45

  @slow
  Scenario: First page of a large shop renders within one second
    Given a logged-in seller owns 5000 orders in one listing
    When the orders page is requested 20 times
    Then the 95th percentile of the orders page render is at most 1000 ms

  Scenario: Status tab filters on the server
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the orders page on the "shipped" tab
    Then every order row shows the status "Đang giao hàng"
    And the orders shown are exactly the SHIPPED orders of the seller
    And the "Đang giao" badge equals the number of SHIPPED orders of the seller

  Scenario: Badges stay stable while searching
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the "shipped" orders filtered by the id prefix of one SHIPPED order
    Then the table shows only the order that matches the prefix
    And the "Tất cả" badge shows 1

  Scenario: Next page uses the returned cursor
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the orders page
    And the seller records the orders shown
    And the seller clicks "Trang sau"
    Then the URL holds a cursor and page 2
    And the orders table shows 20 rows
    And the orders shown share no order with the recorded orders

  Scenario: Last page has no next link
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the orders page
    And the seller goes to page 3 of the orders
    Then the "Trang sau" control is not rendered
    And the orders table shows 5 rows

  Scenario: Query is trimmed to 100 characters
    Given a logged-in seller owns 3 PENDING orders
    When the seller opens "/seller/orders" with a search text of 150 characters
    Then the page renders without an error

  Scenario: Orders page unknown cursor shows the retry state
    Given a logged-in seller owns 3 PENDING orders
    When the seller opens "/seller/orders" with the cursor "not-a-cursor"
    Then the page shows the error Alert "Không tải được danh sách đơn hàng" with a "Thử lại" link to "/seller/orders" without a cursor

  Scenario: Changing tab or search resets the position
    Given a logged-in seller owns 12 PENDING, 8 PAID, 15 SHIPPED and 10 COMPLETED orders
    When the seller opens the orders page
    And the seller goes to page 3 of the orders
    And the seller clicks "Hoàn thành"
    Then the URL contains neither a cursor nor a page
    And the orders shown are only COMPLETED orders
    And the orders table shows 10 rows

  Scenario: No match shows the empty result
    Given a logged-in seller owns 3 PENDING orders
    When the seller searches the orders for "khong-khop-xyz"
    Then the page shows "Không có kết quả" with a "Xoá bộ lọc" link to "/seller/orders"

  Scenario: A non-seller principal is denied
    Given a logged-in buyer without the listing.write scope
    When the buyer opens "/seller/orders"
    Then the page renders no order row and no order tabs

  Scenario: KPIs of a large catalogue
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the dashboard
    Then the KPI "Tổng sản phẩm" shows 45
    And the KPI "Đang bán" shows 30
    And the product table shows 20 rows

  Scenario: Status filter runs in team-domain
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the dashboard with "status=draft"
    Then every product row is a draft listing and all draft listings are shown

  Scenario: Title search runs in team-domain
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the dashboard with "q=áo thun"
    Then every product row title contains "áo thun" ignoring case and all matching listings are shown

  Scenario: Next product page uses the cursor
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the dashboard
    And the seller records the product titles shown
    And the seller clicks "Trang sau"
    Then the URL holds a cursor and page 2
    And the product table shows 20 rows
    And the product table shows listings not shown before

  Scenario: An unknown product cursor shows the retry state
    Given a logged-in seller owns 2 published and 1 draft listings
    When the seller opens "/seller" with the cursor "not-a-cursor"
    Then the product block shows the error Alert with a "Thử lại" link to "/seller" without a cursor

  Scenario: A principal without the seller scope is denied
    Given a logged-in buyer without the listing.write scope
    When the buyer opens "/seller"
    Then the page renders no listing row and no order row

  @destructive
  Scenario: Orders source down degrades only its blocks
    Given a logged-in seller owns 30 published and 15 draft listings
    And the seller also owns 6 SHIPPED orders
    And team-order is stopped
    When the seller opens the dashboard while the orders source is down
    Then the page answered HTTP 200
    And the "Cần gửi" KPI is omitted and the recent-orders block shows an error state with a retry link
    And the product table still renders

  @destructive
  Scenario: Listings source down shows the retry state
    Given a logged-in seller owns 6 PENDING orders
    And team-domain is stopped
    When the seller opens the dashboard while the listings source is down
    Then the product block shows the error Alert "Không tải được danh sách sản phẩm" with a "Thử lại" link
    And the order blocks still render

  Scenario: Picker opens with the first options
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the "ads" page
    Then the picker lists 20 published listings
    And every picker option is an own published listing

  Scenario: Typing narrows the options
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the "bundles" page
    And the seller types "quần" in the picker and waits for the options to settle
    Then the picker lists only own published listings whose title contains "quần" ignoring case

  Scenario: Selection survives a new search
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the "bundles" page
    And the seller selects 2 listings in the picker
    And the seller types "quần" in the picker and waits for the options to settle
    Then the picker lists only own published listings whose title contains "quần" ignoring case
    And the two selected listings remain selected
    When the seller fills the bundle "Combo phân trang" priced 150000 and presses "Tạo combo"
    Then the seller has a stored bundle of exactly the two selected listings

  Scenario: Search without a result
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the "bundles" page
    And the seller types "zzzkhongcosanpham" in the picker and waits for the options to settle
    Then the picker shows "Không tìm thấy sản phẩm" and offers no option

  @destructive
  Scenario: Search fails
    Given a logged-in seller owns 30 published and 15 draft listings
    When the seller opens the "bundles" page
    And the seller selects 1 listings in the picker
    And team-domain is stopped
    And the seller types "quần" in the picker and waits for the options to settle
    Then the picker shows "Không tải được sản phẩm" with a retry control and keeps the current selection

  Scenario: Search text stays out of the server logs
    Given a logged-in seller owns 3 PENDING orders
    When the seller starts the order search "0901234567" and the browser storage is read
    Then the seller app server output since the request does not contain the search text
    And no cookie or browser storage of the seller app contains the search text
