## MODIFIED Requirements

### Requirement: Sellers see what they must ship and by when

The seller dashboard SHALL show a "Cần gửi" KPI counting orders PAID plus orders PENDING with payment method COD (never PENDING online-pay orders), SHALL show "Quá hạn: N" with the cell highlighted when N > 0 orders have `ship_by` strictly before now, and SHALL omit the KPI when the order list cannot be loaded. The seller order list and detail SHALL show "Hạn gửi: <date>" from `ship_by`, with a danger "Quá hạn" tag for a still-shippable order past it.

#### Scenario: Order lifecycle: the to-ship count is paid plus COD pending
- **WHEN** the dashboard KPIs are derived from orders: 2 PAID, 1 PENDING COD, 1 PENDING MOCK_BANK, 1 SHIPPED and 1 CANCELLED
- **THEN** "Cần gửi" is 3 and "Đơn chờ xử lý" is no longer shown
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/kpi.test.ts` `to-ship counts paid and cod pending`

#### Scenario: Order lifecycle: overdue shipments are highlighted
- **WHEN** 3 to-ship orders have `ship_by` of 1 hour ago, exactly now and 1 hour ahead
- **THEN** the cell shows "Quá hạn: 1" (the order at exactly now is not overdue) and carries the danger highlight, and with no overdue order it shows no "Quá hạn" text
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/kpi.test.ts` `overdue is strictly past ship-by`

#### Scenario: Order lifecycle: the to-ship KPI is omitted when orders fail to load
- **WHEN** `ListSellerOrders` fails
- **THEN** the dashboard shows no "Cần gửi" cell and the existing "Không tải được danh sách đơn hàng." alert
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/(dashboard)/page.test.tsx` `to-ship KPI is omitted when orders fail`

#### Scenario: Order lifecycle: seller orders show the ship-by deadline
- **WHEN** the seller order list and detail are rendered for a PAID order with `ship_by` in the past, one with `ship_by` in the future, and a PENDING online order with no `ship_by`
- **THEN** the first shows "Hạn gửi:" with its date and a "Quá hạn" tag, the second shows the date and no tag, the third shows "-" and no tag
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/shipBy.test.tsx` `ship-by shown with overdue tag`

#### Scenario: Order lifecycle: a session without seller scope sees no seller orders
- **WHEN** a signed-in session without scope `listing.write` opens `/seller`
- **THEN** the seller layout renders the onboarding screen (button "Đồng ý và trở thành Người Bán") without rendering its children, so `ListSellerOrders` is never called and no "Cần gửi" cell is shown
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `no seller orders without listing scope`

#### Scenario: Order lifecycle: the seller dashboard flags an overdue order
- **WHEN** the stack runs with the short ship-by of the lifecycle overlay and a seller has a paid order left unshipped past it
- **THEN** the seller dashboard shows "Cần gửi" with "Quá hạn: 1" and the seller order detail shows "Hạn gửi" with the "Quá hạn" tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the seller dashboard flags an overdue order`
