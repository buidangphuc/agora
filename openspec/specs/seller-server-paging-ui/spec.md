# seller-server-paging-ui Specification

## Purpose
Defines how the seller app pages, searches and counts orders and listings through the server-side filters instead of loading and slicing whole lists in Next.js.

## Requirements

### Requirement: Seller orders page is fetched one page at a time

`/seller/orders` SHALL fetch only the displayed page through `OrderService.ListSellerOrders` with `page_size` 20, `statuses` taken from the status tab and `query` taken from the URL `q`, and SHALL render the tab badges from `status_counts` of that same response. The page SHALL NOT call `ListSellerOrders` without `page`. Tab "Chờ xử lý" SHALL map to PENDING and PAID, "Đang giao" to SHIPPED, "Hoàn thành" to COMPLETED and "Tất cả" to no status filter. The page SHALL be reachable only by a principal holding the `listing.write` scope.

#### Scenario: First page of a large shop

- **WHEN** a seller with 5,000 orders opens `/seller/orders`
- **THEN** the table shows 20 rows, the "Tất cả" badge shows 5000, and the seller app issued exactly one `ListSellerOrders` call with `page_size` 20

#### Scenario: Status tab filters on the server

- **WHEN** the seller opens `/seller/orders?status=shipped`
- **THEN** every row has status "Đang giao hàng" and the "Đang giao" badge equals the number of SHIPPED orders of the seller

#### Scenario: Badges stay stable while searching

- **WHEN** the seller opens `/seller/orders?status=shipped&q=<8-char order id prefix>`
- **THEN** the table shows only the matching order and the "Tất cả" badge still shows the count of all orders matching the query

#### Scenario: Next page uses the returned cursor

- **WHEN** the seller clicks "Trang sau" on page 1 of 45 orders
- **THEN** the URL holds `cursor` and `page=2`, the table shows the remaining 20 orders after the first 20, and no order of page 1 appears again

#### Scenario: Last page has no next link

- **WHEN** the seller opens the last page of the list
- **THEN** the "Trang sau" control is not rendered and the table shows between 1 and 20 rows

#### Scenario: Query is trimmed to 100 characters

- **WHEN** the seller opens `/seller/orders?q=` followed by 150 characters
- **THEN** the request carries a `query` of 100 characters and the page renders without an error

#### Scenario: Orders page unknown cursor shows the retry state

- **WHEN** the seller opens `/seller/orders?cursor=not-a-cursor`
- **THEN** the page shows the error Alert "Không tải được danh sách đơn hàng" with a "Thử lại" link to `/seller/orders` without `cursor`

#### Scenario: Changing tab or search resets the position

- **WHEN** the seller is on page 3 and selects the "Hoàn thành" tab
- **THEN** the URL contains neither `cursor` nor `page` and the first page of completed orders is shown

#### Scenario: No match shows the empty result

- **WHEN** the seller searches a text that matches no order
- **THEN** the page shows "Không có kết quả" with a "Xoá bộ lọc" link to `/seller/orders`

#### Scenario: A non-seller principal is denied

- **WHEN** a buyer principal without the `listing.write` scope opens `/seller/orders`
- **THEN** the page renders no order row and no `ListSellerOrders` call is made

### Requirement: Seller dashboard reads bounded data

`/seller` SHALL render the product Table List from `ListingService.ListMyListings` with `page_size` 20, `statuses` from the URL status filter and `query` from the URL `q`, and SHALL NOT filter or slice the listings in the seller app. The KPI "Tổng sản phẩm" SHALL equal `page.total` of an unfiltered `ListMyListings` call and "Đang bán" SHALL equal `page.total` of a `PUBLISHED`-filtered call. The KPI "Cần gửi" SHALL equal the `status_counts` of PENDING plus PAID. The recent-orders block SHALL show the 5 newest orders from one `ListSellerOrders` call with `page_size` 5. The dashboard SHALL make no call that walks cursors.

#### Scenario: KPIs of a large catalogue

- **WHEN** a seller with 2,000 listings (1,200 published) opens `/seller`
- **THEN** "Tổng sản phẩm" shows 2000, "Đang bán" shows 1200 and the product table shows 20 rows

#### Scenario: Status filter runs in team-domain

- **WHEN** the seller opens `/seller?status=draft`
- **THEN** every row is a draft listing and the request carried `statuses = [DRAFT]`

#### Scenario: Title search runs in team-domain

- **WHEN** the seller opens `/seller?q=áo thun`
- **THEN** every row title contains "áo thun" ignoring case and the request carried `query = "áo thun"`

#### Scenario: Next product page uses the cursor

- **WHEN** the seller clicks "Trang sau" under the product table
- **THEN** the URL holds `cursor` and `page=2` and the table shows listings not shown on page 1

#### Scenario: Orders source down degrades only its blocks

- **WHEN** `ListSellerOrders` fails while `/seller` renders
- **THEN** the page answers HTTP 200, the "Cần gửi" KPI is omitted, the recent-orders block shows its error state with a retry link, and the product table still renders

#### Scenario: Listings source down shows the retry state

- **WHEN** `ListMyListings` fails while `/seller` renders
- **THEN** the product block shows the error Alert "Không tải được danh sách sản phẩm" with a "Thử lại" link and the order blocks still render

#### Scenario: An unknown product cursor shows the retry state

- **WHEN** the seller opens `/seller?cursor=not-a-cursor`
- **THEN** the product block shows the error Alert with a "Thử lại" link to `/seller` without `cursor`

#### Scenario: A principal without the seller scope is denied

- **WHEN** a buyer principal without the `listing.write` scope opens `/seller`
- **THEN** the page renders no listing or order row and makes no `ListMyListings` or `ListSellerOrders` call

### Requirement: Listing pickers search on the server

The ads and bundle pickers SHALL load at most 20 own `PUBLISHED` listings per request through `ListMyListings` and SHALL search by typing: after 300 ms without keystrokes the picker SHALL call a server action that sends the trimmed text as `query`. The pickers SHALL NOT call `ListMyListings` more than once per search and SHALL NOT preload the catalogue. The server action SHALL answer only for a principal holding `listing.write`, and a `query` longer than 100 characters SHALL be cut to 100.

#### Scenario: Picker opens with the first options

- **WHEN** a seller with 2,000 listings opens `/seller/ads`
- **THEN** the picker lists at most 20 published listings and the page issued one `ListMyListings` call

#### Scenario: Typing narrows the options

- **WHEN** the seller types "quần" in the bundle picker and waits 300 ms
- **THEN** the picker lists only own published listings whose title contains "quần" ignoring case, at most 20

#### Scenario: Selection survives a new search

- **WHEN** the seller selects two listings in the bundle picker and then searches another text
- **THEN** the two selected listings remain selected and are submitted by "Tạo combo"

#### Scenario: Search without a result

- **WHEN** the seller types a text that matches no published listing
- **THEN** the picker shows "Không tìm thấy sản phẩm" and offers no option

#### Scenario: Search fails

- **WHEN** `ListMyListings` fails during a picker search
- **THEN** the picker shows "Không tải được sản phẩm" with a retry control and keeps the current selection

#### Scenario: A buyer cannot call the search action

- **WHEN** a principal without the `listing.write` scope invokes `searchMyListingsAction`
- **THEN** the action returns an error and no listing is returned
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `searchMyListingsAction denies a principal without listing.write`

### Requirement: Query text is never logged or persisted

The seller app SHALL NOT write the search `q` value, a cursor or an order row field to any log line, cookie or storage; they SHALL exist only in the URL and in the request in flight.

#### Scenario: Search text stays out of the server logs

- **WHEN** a seller searches `/seller/orders?q=0901234567`
- **THEN** the seller app server output captured during the request does not contain "0901234567"
