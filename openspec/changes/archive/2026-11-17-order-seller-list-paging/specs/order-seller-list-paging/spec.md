## ADDED Requirements

### Requirement: ListSellerOrders pages with a keyset cursor

When `ListSellerOrdersRequest.page` is set, `OrderService.ListSellerOrders` SHALL return at most `page_size` of the caller's
own orders ordered by `created_at` descending then id descending, and `page.next_cursor` SHALL be non-empty exactly when more
orders follow. `page_size` 0 SHALL mean 20, values above 100 SHALL be clamped to 100, and a negative value SHALL fail with
INVALID_ARGUMENT. A `cursor` that cannot be decoded SHALL fail with INVALID_ARGUMENT. `page.total` SHALL be the number of the
seller's orders matching `statuses`/`status_filter` and `query`. The seller SHALL come from the principal. Index `orders (seller_id, status, created_at DESC, id DESC)` SHALL exist (migration `0026`, with a down migration that drops it).

#### Scenario: Pages cover the seller's orders exactly once

- **WHEN** a seller owning 250 orders follows `next_cursor` with `page_size = 100` until it is empty
- **THEN** the three pages hold 100, 100 and 50 orders, no order id repeats, `created_at` never increases across pages, and every page reports `page.total` 250

#### Scenario: Default page size is 20

- **WHEN** a seller owning 30 orders calls `ListSellerOrders` with `page = {}` (set, `page_size` 0)
- **THEN** the response holds 20 orders and a non-empty `next_cursor`

#### Scenario: Seller orders: Page size above the maximum is clamped

- **WHEN** a seller owning 150 orders calls `ListSellerOrders` with `page_size = 1000`
- **THEN** the response holds 100 orders and a non-empty `next_cursor`

#### Scenario: A negative page size is rejected

- **WHEN** a seller calls `ListSellerOrders` with `page_size = -1`
- **THEN** the call fails with INVALID_ARGUMENT and no order is returned

#### Scenario: An unknown cursor is rejected

- **WHEN** a seller calls `ListSellerOrders` with `cursor = "not-a-cursor"`
- **THEN** the call fails with INVALID_ARGUMENT and no order is returned

#### Scenario: A cursor never reaches another seller's orders

- **WHEN** seller B reuses a `next_cursor` obtained by seller A while seller A owns 30 orders and seller B owns 2
- **THEN** every returned order belongs to seller B

#### Scenario: A seller with no orders gets an empty page

- **WHEN** a seller owning no order calls `ListSellerOrders` with `page = {}`
- **THEN** the response holds 0 orders, `page.total` is 0, `next_cursor` is empty and `status_counts` holds 5 entries of count 0

#### Scenario: An unauthenticated caller is denied

- **WHEN** a caller without a principal calls `ListSellerOrders`
- **THEN** the call fails with UNAUTHENTICATED and no order is returned

#### Scenario: Paged latency at 5,000 orders

- **WHEN** a seller owning 5,000 orders calls `ListSellerOrders` 50 times with `statuses = [PAID]`, `page_size = 20`
- **THEN** the 95th percentile of the call durations is at most 150 ms

#### Scenario: Migration applies and reverts

- **WHEN** migrations run up to 0026 and down one step on a database holding orders
- **THEN** the index exists after up, is absent after down, and no order row changes
- **VERIFIED BY**: `team-order/migrations/migration_test.go` `TestMigration0026_AppliesAndReverts`

### Requirement: ListSellerOrders filters by status

`ListSellerOrders` SHALL return only orders whose status is in `statuses` when that list is non-empty, SHALL use
`status_filter` when `statuses` is empty and `status_filter` is not UNSPECIFIED, and SHALL return every own order otherwise.
`UNSPECIFIED` inside `statuses` SHALL fail with INVALID_ARGUMENT.

#### Scenario: One status tab

- **WHEN** a seller owning 3 PAID, 2 SHIPPED and 1 CANCELLED order calls `ListSellerOrders` with `statuses = [PAID]` and `page = {}`
- **THEN** the response holds exactly the 3 PAID orders and `page.total` is 3

#### Scenario: Seller orders: Several statuses are OR-ed

- **WHEN** the same seller calls with `statuses = [SHIPPED, CANCELLED]`
- **THEN** the response holds the 2 SHIPPED and the 1 CANCELLED order and `page.total` is 3

#### Scenario: statuses wins over status_filter

- **WHEN** the same seller calls with `status_filter = CANCELLED` and `statuses = [PAID]`
- **THEN** the response holds the 3 PAID orders

#### Scenario: Seller orders: An unspecified status is rejected

- **WHEN** a seller calls with `statuses = [ORDER_STATUS_UNSPECIFIED]`
- **THEN** the call fails with INVALID_ARGUMENT and no order is returned

### Requirement: ListSellerOrders searches by order id prefix or item title

When `query` is non-empty after trimming, `ListSellerOrders` SHALL return only own orders whose id starts with `query` or
having an item whose title contains `query`, both case-insensitive. `%`, `_` and `\` in `query` SHALL match literally. A
`query` longer than 100 runes SHALL fail with INVALID_ARGUMENT. `query` and the status filter SHALL combine with AND. The
`query` value SHALL NOT appear in any log line.

#### Scenario: Item title substring

- **WHEN** a seller with one order containing "Áo thun Đỏ" and another containing "Quần jean" calls with `query = "áo thun"`
- **THEN** the response holds only the first order and `page.total` is 1

#### Scenario: Order id prefix

- **WHEN** a seller calls with `query` equal to the first 8 characters of one of the seller's order ids
- **THEN** the response holds that order and every other own order sharing the prefix, and no order without it unless an item title matches

#### Scenario: Seller orders: Query and status combine

- **WHEN** the seller has a PAID and a SHIPPED order both containing "Áo thun" and calls with `query = "áo"` and `statuses = [SHIPPED]`
- **THEN** the response holds only the SHIPPED order

#### Scenario: Seller orders: LIKE wildcards match literally

- **WHEN** a seller with items "Giảm 50% áo" and "Giảm 500 áo" calls with `query = "50%"`
- **THEN** the response holds only the order with "Giảm 50% áo"

#### Scenario: Seller orders: A query of exactly 100 runes is accepted, 101 is rejected

- **WHEN** a seller calls with a `query` of 100 runes, then with one of 101 runes
- **THEN** the first call succeeds with 0 orders and the second fails with INVALID_ARGUMENT

#### Scenario: Seller orders: A blank query is ignored

- **WHEN** a seller calls with `query = "   "`
- **THEN** the response equals the response for an empty `query`

#### Scenario: Seller orders: No match returns an empty page

- **WHEN** a seller calls with `query = "zzzz-no-such-title"`
- **THEN** the response holds 0 orders, `page.total` is 0 and `next_cursor` is empty

#### Scenario: Another seller's orders are never returned

- **WHEN** seller B, owning no order matching, calls with the exact title or id of seller A's order as `query`
- **THEN** the response holds 0 orders and `page.total` is 0

#### Scenario: Seller orders: The query text is not logged

- **WHEN** a seller calls with `query = "secret-needle-title"`
- **THEN** no log line of `team-order-svc` for that call contains "secret-needle-title"

### Requirement: ListSellerOrders returns stable status counts

Every `ListSellerOrders` response SHALL carry `status_counts` with one entry for each of PENDING, PAID, SHIPPED, COMPLETED
and CANCELLED in that order, counting all the seller's orders with `query` applied and `statuses`/`status_filter` ignored.

#### Scenario: Counts ignore the selected tab

- **WHEN** a seller owning 3 PAID, 2 SHIPPED and 1 CANCELLED order calls with `statuses = [PAID]`
- **THEN** `status_counts` is PENDING 0, PAID 3, SHIPPED 2, COMPLETED 0, CANCELLED 1

#### Scenario: Counts never include another seller's orders

- **WHEN** seller B owning 2 PAID orders calls while seller A owns 3 PAID orders
- **THEN** `status_counts` for seller B shows PAID 2 and no count from seller A

#### Scenario: A rejected request returns no counts

- **WHEN** a seller calls with `statuses = [ORDER_STATUS_UNSPECIFIED]`
- **THEN** the call fails with INVALID_ARGUMENT and the response carries no `status_counts`

#### Scenario: Counts follow the query

- **WHEN** the same seller calls with a `query` matching one PAID and one SHIPPED order
- **THEN** `status_counts` is PENDING 0, PAID 1, SHIPPED 1, COMPLETED 0, CANCELLED 0

### Requirement: Unset page keeps the full-list behaviour

When `page` is not set, `ListSellerOrders` SHALL return all matching own orders ordered by `created_at` descending, as before
this change, with empty `next_cursor`, `page.total` equal to the number of orders returned, and `status_counts` filled.

#### Scenario: Legacy call returns the full list

- **WHEN** a seller owning 250 orders calls `ListSellerOrders` with only `status_filter = PAID` and no `page`, 120 orders being PAID
- **THEN** the response holds all 120 PAID orders newest first, `next_cursor` is empty and `page.total` is 120

#### Scenario: Legacy call without any field

- **WHEN** a seller owning 6 orders calls `ListSellerOrders` with an empty request
- **THEN** the response holds all 6 orders newest first
