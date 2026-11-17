## ADDED Requirements

### Requirement: The seller selects shippable orders on the order list

`/seller/orders` SHALL render a checkbox on every row whose order is PAID, or PENDING with payment method COD, a select-all checkbox for the current page, and a selection bar showing the selected count with "Gửi hàng loạt" and "In phiếu hàng loạt". Rows in any other status SHALL have no checkbox. The selection SHALL hold at most 50 orders; selecting a 51st SHALL be refused with the message "Chọn tối đa 50 đơn" and leave the selection unchanged. With nothing selected the bar's two buttons SHALL be disabled.

#### Scenario: Bulk ship: only shippable rows can be selected
- **WHEN** a seller opens `/seller/orders` with one PAID, one PENDING COD, one PENDING online-payment, one SHIPPED and one CANCELLED order on the page
- **THEN** exactly the PAID and the PENDING COD rows carry a checkbox
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerOrdersTable.test.tsx` `checkbox only on shippable rows`

#### Scenario: Bulk ship: select-all selects the shippable rows of the page
- **WHEN** the seller ticks the select-all checkbox on a page holding 8 shippable and 12 other rows
- **THEN** the selection bar shows "8" selected and both buttons are enabled
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/BulkShipBar.test.tsx` `select all picks shippable rows`

#### Scenario: Bulk ship: nothing selected disables both buttons
- **WHEN** the seller opens the order list without selecting any row
- **THEN** "Gửi hàng loạt" and "In phiếu hàng loạt" are disabled
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/BulkShipBar.test.tsx` `buttons disabled when empty`

#### Scenario: Bulk ship: the fifty-first selection is refused
- **WHEN** 50 orders are selected and the seller ticks one more
- **THEN** the message "Chọn tối đa 50 đơn" shows and the selection count stays 50
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/bulkShip.test.ts` `selection stops at 50`

### Requirement: Bulk ship moves each selected order to SHIPPED and reports per-order results

Confirming "Gửi hàng loạt" SHALL call `OrderService.UpdateOrderStatus` once per selected order with status SHIPPED and the optional tracking number typed for that order (trimmed; omitted when empty), with at most 4 calls in flight. The result list SHALL show one row per order with success or a fixed Vietnamese reason mapped from the Connect code. A failure of one order SHALL NOT stop or roll back the others. Orders that succeeded SHALL leave the selection; failed orders SHALL stay selected. The seller app SHALL NOT invent tracking numbers. The action SHALL stop starting new calls 15 s after the batch began and report each unstarted order as not attempted.

#### Scenario: Bulk ship: three paid orders ship with tracking numbers
- **WHEN** the seller selects 3 PAID orders, types tracking numbers "T1" and "T2" for the first two and leaves the third empty, and confirms
- **THEN** `UpdateOrderStatus` is called 3 times with status SHIPPED and tracking "T1", "T2" and "" and the result list shows 3 successes
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `bulk ship sends status and tracking per order`

#### Scenario: Bulk ship: one order cancelled meanwhile fails only its row
- **WHEN** 3 orders are confirmed and `UpdateOrderStatus` answers FAILED_PRECONDITION for the second
- **THEN** the result list shows rows 1 and 3 as success and row 2 as "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)", and the other two calls still happened
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `one failure does not stop the others`

#### Scenario: Bulk ship: another seller's order is denied per row
- **WHEN** the input contains an order of another seller and `UpdateOrderStatus` answers PERMISSION_DENIED for it
- **THEN** that row shows "Không có quyền với đơn này", no other row is affected, and no upstream error text is shown
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `foreign order is denied per row`

#### Scenario: Bulk ship: at most four calls are in flight
- **WHEN** 50 orders are confirmed and each `UpdateOrderStatus` takes 100 ms
- **THEN** the observed maximum of concurrent calls is 4 and all 50 results come back in input order
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/bulkShip.test.ts` `runner caps concurrency at 4 and keeps order`

#### Scenario: Bulk ship: orders not started within 15 seconds are reported as not attempted
- **WHEN** the batch has run for 15 s with 10 orders not yet started
- **THEN** those 10 rows show "Chưa xử lý, hãy thử lại", no further call is started, and the 40 earlier rows keep their own results
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/bulkShip.test.ts` `deadline marks unstarted as not attempted`

#### Scenario: Bulk ship: failed rows stay selected and shipped rows leave
- **WHEN** a batch of 3 ends with row 2 failed
- **THEN** the selection holds only order 2 and the list is revalidated once
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/BulkShipModal.test.tsx` `failed rows stay selected`

#### Scenario: Bulk ship: a team-order outage fails rows with a reason
- **WHEN** `UpdateOrderStatus` answers UNAVAILABLE for every order of a batch of 2
- **THEN** both rows show "Dịch vụ đơn hàng đang bận" and the modal stays open with the retry option
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `unavailable maps to a busy reason`

### Requirement: The bulk ship action validates and authorises before any upstream call

`bulkShipOrdersAction` SHALL require a session principal with scope `listing.write` and SHALL accept 1 to 50 distinct non-empty order ids. Any violation SHALL return an error result and SHALL make no `UpdateOrderStatus` call. Duplicate ids SHALL be collapsed to one call. Ownership SHALL be decided only by team-order.

#### Scenario: Bulk ship: more than 50 ids are rejected before any call
- **WHEN** the action receives 51 distinct order ids
- **THEN** it returns an error "Chọn tối đa 50 đơn" and `UpdateOrderStatus` is never called
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `51 ids rejected without calls`

#### Scenario: Bulk ship: an empty batch is rejected
- **WHEN** the action receives an empty list
- **THEN** it returns an error and `UpdateOrderStatus` is never called
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `empty batch rejected`

#### Scenario: Bulk ship: duplicate ids ship once
- **WHEN** the action receives the same order id twice
- **THEN** `UpdateOrderStatus` is called once and the result list has one row for it
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `duplicate ids collapse`

#### Scenario: Bulk ship: a principal without listing.write is denied
- **WHEN** the action runs for an anonymous visitor, and again for a signed-in principal without `listing.write`
- **THEN** both get an error result and `UpdateOrderStatus` is never called
- **VERIFIED BY**: `team-frontend-seller/src/features/order/actions.test.ts` `scope required before any call`

### Requirement: Batch print renders one packing slip per selected order

"In phiếu hàng loạt" SHALL open `/seller/orders/print?ids=<ids>`; the route SHALL require a session principal with `listing.write`, accept 1 to 50 distinct ids, read each order with `OrderService.GetOrder` (at most 4 in flight) and render one packing slip per readable order, each followed by a page break, in the order of the ids. An order that cannot be read SHALL produce no slip and SHALL be named in a notice that does not print. The route SHALL be `force-dynamic`, send `Cache-Control: no-store`, be noindex, and log no slip data.

#### Scenario: Bulk ship: three selected orders print three slips
- **WHEN** the seller selects 3 orders and opens "In phiếu hàng loạt"
- **THEN** `/seller/orders/print` renders exactly 3 packing slips in the order of the ids, each with recipient name, phone, address, items and tracking number
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/print/page.test.tsx` `renders one slip per order in id order`

#### Scenario: Bulk ship: an unreadable order is named and skipped
- **WHEN** `GetOrder` answers PERMISSION_DENIED for one of 3 ids
- **THEN** 2 slips render and a screen-only notice "Không in được" names the third id
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/print/page.test.tsx` `unreadable order named not printed`

#### Scenario: Bulk ship: more than 50 print ids are rejected
- **WHEN** `/seller/orders/print` is requested with 51 distinct ids
- **THEN** the page shows an error and `GetOrder` is never called
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/print/page.test.tsx` `51 ids rejected without calls`

#### Scenario: Bulk ship: a malformed or empty ids parameter is rejected
- **WHEN** `/seller/orders/print` is requested with no `ids` or with an empty one
- **THEN** the page shows an error and `GetOrder` is never called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/bulkShip.test.ts` `parseIds rejects empty and malformed`

#### Scenario: Bulk ship: an anonymous visitor or a non-seller cannot print
- **WHEN** `/seller/orders/print` is requested by an anonymous visitor, and again by a principal without `listing.write`
- **THEN** the visitor is redirected to login, the other principal gets no slips, and `GetOrder` is never called in either case
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/print/page.test.tsx` `no slips and no calls without seller scope`

#### Scenario: Bulk ship: the print page is not cached or indexed
- **WHEN** `/seller/orders/print` is rendered for a seller
- **THEN** the response has `Cache-Control: no-store` and robots noindex
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/print/page.test.tsx` `no-store and noindex`

### Requirement: Bulk ship and batch print are verified in the browser against the real services

Bulk ship and batch print SHALL work against the real gateway and team-order: shipped orders SHALL show as "Đang giao" after the batch and the batch print SHALL list the selected orders' buyers.

#### Scenario: Bulk ship: a seller ships several paid orders in the browser
- **WHEN** a seller with 3 PAID orders selects all three on `/seller/orders`, enters a tracking number for one and confirms
- **THEN** the result list shows 3 successes and the three orders show status "Đang giao" on the list, the one tracking number on its detail page
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_bulk_ship.feature` `Bulk ship: a seller ships several paid orders in the browser`

#### Scenario: Bulk ship: an order cancelled meanwhile fails only its row in the browser
- **WHEN** a seller selects 3 PAID orders, one of them is cancelled by its buyer, and the seller confirms
- **THEN** the cancelled order's row shows its failure reason and the other two orders become "Đang giao"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_bulk_ship.feature` `Bulk ship: an order cancelled meanwhile fails only its row in the browser`

#### Scenario: Bulk ship: batch print shows one slip per selected order in the browser
- **WHEN** a seller selects 2 orders and clicks "In phiếu hàng loạt"
- **THEN** the print page shows 2 packing slips with the two buyers' recipient names
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_bulk_ship.feature` `Bulk ship: batch print shows one slip per selected order in the browser`

#### Scenario: Bulk ship: another seller cannot ship or print a foreign order in the browser
- **WHEN** seller B opens `/seller/orders/print?ids=` with an order id of seller A
- **THEN** the page shows no slip and names the order in the "Không in được" notice
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_bulk_ship.feature` `Bulk ship: another seller cannot ship or print a foreign order in the browser`
