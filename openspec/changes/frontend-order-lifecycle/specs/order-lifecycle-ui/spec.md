## ADDED Requirements

### Requirement: Cancel is offered only where the backend allows it and sends a reason code

The buyer order detail SHALL offer "Hủy đơn" only for PENDING and PAID orders; a SHIPPED order SHALL offer "Yêu cầu trả hàng" and "Đã nhận được hàng" instead, and COMPLETED and CANCELLED orders SHALL offer no cancel. The cancel dialog SHALL send the label and a `CancelReason` code chosen from a fixed table (`changed_mind`, `wrong_item`, `damaged`, `delivery_delay`); the UI SHALL NOT be able to send `CANCEL_REASON_PAYMENT_EXPIRED` or `CANCEL_REASON_ADMIN_FORCE_FAIL`.

#### Scenario: Order lifecycle: cancel is offered only for pending and paid orders
- **WHEN** the buyer order detail is rendered for orders in PENDING, PAID, SHIPPED, COMPLETED and CANCELLED
- **THEN** "Hủy đơn" is present for exactly PENDING and PAID and absent for the other three
- **VERIFIED BY**: `team-frontend/src/features/order/OrderDetailView.test.tsx` `cancel only for pending and paid`

#### Scenario: Order lifecycle: a shipped order offers receipt and return instead of cancel
- **WHEN** the buyer order detail is rendered for a SHIPPED order
- **THEN** "Hủy đơn" is absent and "Yêu cầu trả hàng" and "Đã nhận được hàng" are present
- **VERIFIED BY**: `team-frontend/src/features/order/OrderDetailView.test.tsx` `shipped offers receipt and return`

#### Scenario: Order lifecycle: cancel sends the reason code with the label
- **WHEN** `cancelOrderAction("o1", "wrong_item")` runs
- **THEN** `cancelOrder` is called once with id "o1", the label of `wrong_item` and `CancelReason.WRONG_ITEM`, and the buyer list and detail paths are revalidated
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `cancel sends the reason code`

#### Scenario: Order lifecycle: an unknown cancel reason is refused before any call
- **WHEN** `cancelOrderAction` receives "payment_expired", "admin_force_fail" or any value outside the four options
- **THEN** it returns `{ ok: false, error: "Lý do hủy không hợp lệ." }` and `cancelOrder` is never called
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `unknown cancel reason is refused`

#### Scenario: Order lifecycle: a refused cancel keeps the dialog open with the server error
- **WHEN** the buyer confirms a cancel and the server answers `FAILED_PRECONDITION` with a message
- **THEN** an error toast shows that message, the dialog stays open and "Xác nhận hủy đơn" is enabled again
- **VERIFIED BY**: `team-frontend/src/features/order/OrderActions.test.tsx` `a refused cancel keeps the dialog open`

#### Scenario: Order lifecycle: cancelling an order that is not the buyer's is refused
- **WHEN** a buyer submits `cancelOrderAction` for another buyer's order and the server answers `PERMISSION_DENIED`
- **THEN** the action returns `{ ok: false }` with the error message and revalidates nothing
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `cancel of a foreign order is refused`

#### Scenario: Order lifecycle: the buyer cancels a paid order with a reason
- **WHEN** a buyer cancels a PAID order choosing "Sản phẩm bị lỗi / hư hỏng" in the dialog
- **THEN** the detail shows "Đã hủy" and `GetOrder` returns `cancel_reason = CANCEL_REASON_DAMAGED`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the buyer cancels a paid order with a reason`

### Requirement: Payment deadline and retry

An online-pay PENDING order with `payment_due_at` SHALL show "Hạn thanh toán: <date time>" and, while `now < payment_due_at`, a "Thanh toán lại" link to `/checkout/pay/{id}` on the order list and the order detail. At or after `payment_due_at` the link SHALL be replaced by "Đã hết hạn thanh toán". A COD order SHALL show neither. The pay page SHALL open a transaction when none exists (`CreatePayment`), reuse the existing FAILED transaction, show "Đơn hàng đã hết hạn thanh toán" with no pay action for an expired order, render a non-PENDING order's state (paid or cancelled) without any payment call, never make a call for an order the buyer cannot read, and never state that a failed payment cancelled the order.

#### Scenario: Order lifecycle: an unpaid online order shows its deadline and the retry link
- **WHEN** the order list and the order detail are rendered for a PENDING MOCK_MOMO order with `payment_due_at` one hour after now
- **THEN** each shows "Hạn thanh toán:" with that time and a "Thanh toán lại" link whose href is `/checkout/pay/{id}`, and neither contains "Thanh toán ngay"
- **VERIFIED BY**: `team-frontend/src/features/order/OrderList.test.tsx` `unpaid online order shows deadline and retry link`; `team-frontend/src/features/order/OrderDetailView.test.tsx` `unpaid online order shows deadline and retry link`

#### Scenario: Order lifecycle: a COD order shows no deadline and no pay action
- **WHEN** the order list and detail are rendered for a PENDING COD order
- **THEN** neither contains "Hạn thanh toán" nor "Thanh toán lại" nor "Đã hết hạn thanh toán"
- **VERIFIED BY**: `team-frontend/src/features/order/OrderList.test.tsx` `cod order has no deadline or pay action`

#### Scenario: Order lifecycle: the pay action disappears at the deadline instant
- **WHEN** the order list is rendered for a PENDING online order at 1 ms before `payment_due_at` and again at exactly `payment_due_at`
- **THEN** the first shows "Thanh toán lại" and the second shows "Đã hết hạn thanh toán" and no "Thanh toán lại"
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `canPayAgain flips at the deadline instant`; `team-frontend/src/features/order/OrderList.test.tsx` `pay link becomes expiry text at the deadline instant`

#### Scenario: Order lifecycle: an order with no deadline keeps its pay action
- **WHEN** a PENDING online order has no `payment_due_at` (placed before the lifecycle release)
- **THEN** `canPayAgain` is true and no "Hạn thanh toán" text is shown
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `legacy order without deadline can still be paid`; `team-frontend/src/features/order/OrderList.test.tsx` `legacy order without deadline shows no deadline text`; `team-frontend/src/features/order/OrderDetailView.test.tsx` `legacy order without deadline shows no deadline text`

#### Scenario: Order lifecycle: the pay page opens a transaction when none exists
- **WHEN** the buyer opens `/checkout/pay/{id}` of a PENDING MOCK_MOMO order before its deadline and `GetPayment` finds no transaction
- **THEN** `createPayment` is called once with the order id and `MOCK_MOMO` and the page shows the payment simulator for the created transaction instead of a 404
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `creates a transaction when none exists`

#### Scenario: Order lifecycle: the pay page reuses a failed transaction
- **WHEN** the buyer opens the pay page of a PENDING order whose transaction is FAILED and the deadline is in the future
- **THEN** `createPayment` is not called, the failed phase shows "Đơn hàng vẫn được giữ đến" with the deadline, and the retry button is present
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `failed transaction is reused and the order is kept`

#### Scenario: Order lifecycle: an expired order shows the expiry message and no pay action
- **WHEN** the buyer opens the pay page of an order that is CANCELLED with `PAYMENT_EXPIRED`, and again of a PENDING order whose `payment_due_at` is in the past
- **THEN** both show "Đơn hàng đã hết hạn thanh toán", neither shows "Thanh toán thành công" or "Thanh toán thất bại" buttons, and `createPayment` and `getPayment` are not called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `expired order shows the expiry message`

#### Scenario: Order lifecycle: a COD order on the pay page offers no payment
- **WHEN** the buyer opens the pay page of a PENDING COD order
- **THEN** the page shows "Đơn hàng thanh toán khi nhận hàng" and no payment buttons, and `createPayment` is not called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `cod order offers no online payment`

#### Scenario: Order lifecycle: the pay page makes no call for another buyer's order
- **WHEN** the buyer opens the pay page and `GetOrder` refuses the order with 403 or 404 because the order is not the buyer's
- **THEN** the pay page renders not-found and `getPayment` and `createPayment` are never called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `unreadable order makes no payment call`

#### Scenario: Order lifecycle: the pay page of a non-pending order makes no payment call
- **WHEN** the buyer opens the pay page of a PAID, a SHIPPED and a COMPLETED order, and of an order CANCELLED with `CANCEL_REASON_CHANGED_MIND` that never had a transaction
- **THEN** the first three show "Đơn hàng đã được thanh toán" with a "Xem đơn hàng" link to `/account/orders/{id}`; the cancelled one shows "Đơn hàng đã bị hủy" and "Đơn hàng đã bị hủy và tồn kho đã được giải phóng." instead of a 404; none shows payment buttons; the card shows the order's total and payment method; `getPayment` and `createPayment` are never called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `non-pending order renders its state without payment calls`

#### Scenario: Order lifecycle: a payment refused after expiry refreshes the page
- **WHEN** the buyer presses "Thanh toán thành công" and `processMockPaymentAction` fails because the order expired meanwhile
- **THEN** an error toast shows and `router.refresh()` is called once so the server renders the expiry state
- **VERIFIED BY**: `team-frontend/src/features/payment/PaymentSimulator.test.tsx` `a failed payment refreshes the page`

#### Scenario: Order lifecycle: a failed payment does not claim the order was cancelled
- **WHEN** the pay page is rendered for a PENDING online order with a FAILED transaction before its deadline
- **THEN** the page does not contain "đơn hàng đã bị hủy" nor "hoàn tác"
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `no false cancellation claim`

#### Scenario: Order lifecycle: an expired order detail names the expiry
- **WHEN** the buyer order detail is rendered for a CANCELLED order with `cancel_reason = PAYMENT_EXPIRED`, and for one with `CANCEL_REASON_CHANGED_MIND`
- **THEN** the first shows "Đơn hàng đã hết hạn thanh toán và được hệ thống hủy tự động." and the second shows the existing "Tồn kho sản phẩm đã được hoàn lại cho người bán."
- **VERIFIED BY**: `team-frontend/src/features/order/OrderDetailView.test.tsx` `expired cancel names the expiry`

#### Scenario: Order lifecycle: a failed payment can be retried from the order list
- **WHEN** a buyer places a MOCK_MOMO order, fails the payment, opens the order list, presses "Thanh toán lại" and pays successfully
- **THEN** the pay page opens with the simulator, and after success the order shows "Đã thanh toán" and `GetOrder` returns status PAID
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: a failed payment can be retried from the order list`

#### Scenario: Order lifecycle: the pay page opens for an order that has no transaction
- **WHEN** a buyer's online order exists with no payment transaction (created through `CreateOrder` without `CreatePayment`) and the buyer opens its pay page
- **THEN** the page returns 200 with the payment simulator and `GetPayment` by order id now returns a PENDING transaction
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the pay page opens for an order that has no transaction`

#### Scenario: Order lifecycle: an unpaid order expires and the pay page says so
- **WHEN** the stack runs with the short payment deadline of the lifecycle overlay and a buyer's online order stays unpaid past its deadline
- **THEN** within the sweep interval the order list shows "Đã hủy", its pay page shows "Đơn hàng đã hết hạn thanh toán" with no payment buttons, and `GetOrder` returns `cancel_reason = CANCEL_REASON_PAYMENT_EXPIRED`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: an unpaid order expires and the pay page says so`

### Requirement: The buyer confirms receipt of a shipped order

A SHIPPED order SHALL offer "Đã nhận được hàng" on the buyer detail and list, also while its return request is PENDING or APPROVED. Confirming SHALL call `ConfirmReceipt`, then show the order COMPLETED with the review buttons. IF the order is no longer SHIPPED THEN the UI SHALL show "Đơn hàng đã được hoàn thành." and refresh. A SHIPPED order with `auto_complete_at` SHALL state the automatic completion time.

#### Scenario: Order lifecycle: a shipped order row offers receipt on the buyer list
- **WHEN** the buyer order list is rendered with one SHIPPED, one PAID and one COMPLETED order
- **THEN** only the SHIPPED row contains "Đã nhận được hàng"
- **VERIFIED BY**: `team-frontend/src/features/order/OrderList.test.tsx` `shipped row offers receipt`

#### Scenario: Order lifecycle: a shipped order with an open return still offers receipt
- **WHEN** the buyer order detail is rendered for a SHIPPED order whose return request is PENDING, and for one whose return is APPROVED
- **THEN** both contain "Đã nhận được hàng" (the server allows `ConfirmReceipt` while a return is PENDING or APPROVED)
- **VERIFIED BY**: `team-frontend/src/features/order/OrderDetailView.test.tsx` `receipt offered while a return is open`

#### Scenario: Order lifecycle: confirming receipt calls the server and completes the order
- **WHEN** the buyer opens "Đã nhận được hàng" on a SHIPPED order and confirms in the dialog
- **THEN** `confirmReceipt` is called once with the order id, a success toast "Đã xác nhận nhận hàng." shows, and the buyer list and detail paths are revalidated
- **VERIFIED BY**: `team-frontend/src/features/order/ConfirmReceiptButton.test.tsx` `confirm calls ConfirmReceipt and revalidates`

#### Scenario: Order lifecycle: leaving the dialog sends nothing
- **WHEN** the buyer opens the receipt dialog and presses "Chưa" or closes it
- **THEN** `confirmReceipt` is not called and the order status is unchanged
- **VERIFIED BY**: `team-frontend/src/features/order/ConfirmReceiptButton.test.tsx` `cancelling the dialog sends nothing`

#### Scenario: Order lifecycle: a confirm that lost to auto-complete is explained
- **WHEN** `confirmReceiptAction` gets `FAILED_PRECONDITION` from the server
- **THEN** it returns the error "Đơn hàng đã được hoàn thành." and revalidates the buyer list and detail paths
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `confirm after auto-complete is explained`

#### Scenario: Order lifecycle: a refused confirm shows the server error
- **WHEN** `confirmReceiptAction` gets `PERMISSION_DENIED` or any other error
- **THEN** it returns `{ ok: false }` with the error message and does not revalidate
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `refused confirm returns the server error`

#### Scenario: Order lifecycle: only the buyer view offers the receipt button
- **WHEN** the seller (not the buyer, a 403 case on the server) opens the seller order detail of a SHIPPED order
- **THEN** it does not contain "Đã nhận được hàng"
- **VERIFIED BY**: `team-frontend/src/app/(shop)/seller/orders/[id]/page.test.tsx` `seller view has no receipt button`

#### Scenario: Order lifecycle: a shipped order states its automatic completion time
- **WHEN** the detail is rendered for a SHIPPED order with `auto_complete_at` set, and for one without it
- **THEN** the first contains "Tự động hoàn thành vào" with that date and the second contains no such text
- **VERIFIED BY**: `team-frontend/src/features/order/OrderDetailView.test.tsx` `shipped shows the auto-complete time when set`

#### Scenario: Order lifecycle: the buyer confirms receipt and the review prompt opens
- **WHEN** the seller ships a paid order and the buyer opens its detail and confirms "Đã nhận được hàng"
- **THEN** the detail shows "Đã hoàn thành" and an "Đánh giá" button, and `GetOrder` returns status COMPLETED with `completed_at` set
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the buyer confirms receipt and the review prompt opens`

#### Scenario: Order lifecycle: the system completes a shipped order when the buyer does not
- **WHEN** the stack runs with the short auto-complete time of the lifecycle overlay and a shipped order is left alone
- **THEN** within the sweep interval after that time the buyer detail shows "Đã hoàn thành" with an "Đánh giá" button and no "Đã nhận được hàng"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the system completes a shipped order when the buyer does not`

### Requirement: Returns follow the return window

The buyer SHALL be offered "Yêu cầu trả hàng" for a SHIPPED order and for a COMPLETED order while `now < return_window_ends_at`, and SHALL NOT be offered it for a PAID order or from the instant `return_window_ends_at` is reached. A COMPLETED order with a window SHALL show "Có thể trả hàng đến <date>" while open and "Đã hết thời hạn trả hàng" after. A server refusal "return window closed" SHALL be shown as a fixed Vietnamese message.

#### Scenario: Order lifecycle: a return is offered for shipped orders and inside the window
- **WHEN** `canReturn` is evaluated for a SHIPPED order and for a COMPLETED order whose `return_window_ends_at` is one day after now
- **THEN** it is true for both and the detail shows "Yêu cầu trả hàng" and, for the completed one, "Có thể trả hàng đến"
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `return offered when shipped or inside the window`; `team-frontend/src/features/order/OrderDetailView.test.tsx` `return action and window text inside the window`

#### Scenario: Order lifecycle: a return is not offered on a paid order
- **WHEN** `canReturn` is evaluated for PAID, PENDING and CANCELLED orders
- **THEN** it is false for all three
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `no return before shipping`

#### Scenario: Order lifecycle: the return window closes at its end instant
- **WHEN** `canReturn` is evaluated for a COMPLETED order at 1 ms before `return_window_ends_at` and at exactly `return_window_ends_at`
- **THEN** it is true then false, and the detail at the second instant shows "Đã hết thời hạn trả hàng" and no "Yêu cầu trả hàng"
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `return window closes at its end instant`; `team-frontend/src/features/order/OrderDetailView.test.tsx` `window closed text replaces the return action`

#### Scenario: Order lifecycle: a completed order without a window leaves the decision to the server
- **WHEN** `canReturn` is evaluated for a COMPLETED order with no `return_window_ends_at`
- **THEN** it is true and no "Có thể trả hàng đến" text is shown
- **VERIFIED BY**: `team-frontend/src/features/order/lifecycle.test.ts` `completed order without window is offered`; `team-frontend/src/features/order/OrderDetailView.test.tsx` `completed order without window shows no return deadline`

#### Scenario: Order lifecycle: a window-closed refusal is explained
- **WHEN** `createReturnRequestAction` gets `FAILED_PRECONDITION` with message "return window closed"
- **THEN** it returns the error "Đã hết thời hạn trả hàng (15 ngày kể từ khi đơn hoàn thành)." and for any other `FAILED_PRECONDITION` returns that server message unchanged
- **VERIFIED BY**: `team-frontend/src/features/order/returns.actions.test.ts` `window closed refusal is explained`

#### Scenario: Order lifecycle: a return on an order that is not the buyer's is refused
- **WHEN** `createReturnRequestAction` for an order that is not the owner's gets `PERMISSION_DENIED` (403) from the server
- **THEN** it returns `{ ok: false }` with the server message and the returns list is not revalidated
- **VERIFIED BY**: `team-frontend/src/features/order/returns.actions.test.ts` `foreign order return is refused`

#### Scenario: Order lifecycle: a return is accepted on a shipped order
- **WHEN** a buyer opens the detail of a SHIPPED order and submits a return with a reason
- **THEN** the modal closes and the returns tab shows a "Chờ duyệt" badge, and `ListOrderReturns` returns one PENDING return
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: a return is accepted on a shipped order`

#### Scenario: Order lifecycle: after the return window no return is offered
- **WHEN** the stack runs with the short return window of the lifecycle overlay and a buyer opens a completed order after that window
- **THEN** the detail shows "Đã hết thời hạn trả hàng" and no "Yêu cầu trả hàng", and a `CreateReturnRequest` for it through the gateway is refused with `FAILED_PRECONDITION`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: after the return window no return is offered`

### Requirement: Sellers see what they must ship and by when

The seller dashboard SHALL show a "Cần gửi" KPI counting orders PAID plus orders PENDING with payment method COD (never PENDING online-pay orders), SHALL show "Quá hạn: N" with the cell highlighted when N > 0 orders have `ship_by` strictly before now, and SHALL omit the KPI when the order list cannot be loaded. The seller order list and detail SHALL show "Hạn gửi: <date>" from `ship_by`, with a danger "Quá hạn" tag for a still-shippable order past it.

#### Scenario: Order lifecycle: the to-ship count is paid plus COD pending
- **WHEN** the dashboard KPIs are derived from orders: 2 PAID, 1 PENDING COD, 1 PENDING MOCK_BANK, 1 SHIPPED and 1 CANCELLED
- **THEN** "Cần gửi" is 3 and "Đơn chờ xử lý" is no longer shown
- **VERIFIED BY**: `team-frontend/src/features/seller/kpi.test.ts` `to-ship counts paid and cod pending`

#### Scenario: Order lifecycle: overdue shipments are highlighted
- **WHEN** 3 to-ship orders have `ship_by` of 1 hour ago, exactly now and 1 hour ahead
- **THEN** the cell shows "Quá hạn: 1" (the order at exactly now is not overdue) and carries the danger highlight, and with no overdue order it shows no "Quá hạn" text
- **VERIFIED BY**: `team-frontend/src/features/seller/kpi.test.ts` `overdue is strictly past ship-by`

#### Scenario: Order lifecycle: the to-ship KPI is omitted when orders fail to load
- **WHEN** `ListSellerOrders` fails
- **THEN** the dashboard shows no "Cần gửi" cell and the existing "Không tải được danh sách đơn hàng." alert
- **VERIFIED BY**: `team-frontend/src/app/(shop)/seller/(dashboard)/page.test.tsx` `to-ship KPI is omitted when orders fail`

#### Scenario: Order lifecycle: seller orders show the ship-by deadline
- **WHEN** the seller order list and detail are rendered for a PAID order with `ship_by` in the past, one with `ship_by` in the future, and a PENDING online order with no `ship_by`
- **THEN** the first shows "Hạn gửi:" with its date and a "Quá hạn" tag, the second shows the date and no tag, the third shows "-" and no tag
- **VERIFIED BY**: `team-frontend/src/features/seller/shipBy.test.tsx` `ship-by shown with overdue tag`

#### Scenario: Order lifecycle: a session without seller scope sees no seller orders
- **WHEN** a signed-in session without scope `listing.write` opens `/seller`
- **THEN** the seller layout renders "Cần tài khoản Người Bán" without rendering its children, so `ListSellerOrders` is never called and no "Cần gửi" cell is shown
- **VERIFIED BY**: `team-frontend/src/app/(shop)/seller/layout.test.tsx` `no seller orders without listing scope`

#### Scenario: Order lifecycle: the seller dashboard flags an overdue order
- **WHEN** the stack runs with the short ship-by of the lifecycle overlay and a seller has a paid order left unshipped past it
- **THEN** the seller dashboard shows "Cần gửi" with "Quá hạn: 1" and the seller order detail shows "Hạn gửi" with the "Quá hạn" tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the seller dashboard flags an overdue order`

### Requirement: Lifecycle notifications open real pages

The notification shown to a seller for a new order SHALL open `/seller/orders/{id}` on the seller's own order, and the pay reminder shown to a buyer SHALL open `/checkout/pay/{id}` on that buyer's own unpaid order; no UI code change is needed beyond rendering `link_url`.

#### Scenario: Order lifecycle: the new-order notification opens the seller order
- **WHEN** a buyer places an order and the seller opens the notifications page and follows "Đơn hàng mới"
- **THEN** the seller lands on `/seller/orders/{id}` showing that order with status 200
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the new-order notification opens the seller order`

#### Scenario: Order lifecycle: another seller cannot open the new-order link
- **WHEN** a different seller opens `/seller/orders/{id}` of an order that is not theirs, taken from the first seller's notification
- **THEN** the response is the not-found page (404) and shows no order data
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: another seller cannot open the new-order link`

#### Scenario: Order lifecycle: the payment reminder opens the pay page
- **WHEN** the stack runs with the short reminder lead of the lifecycle overlay and a buyer's online order nears its deadline
- **THEN** the buyer's notifications page shows one payment reminder whose link opens `/checkout/pay/{id}` with the payment simulator visible
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/order_lifecycle_ui.feature` `Order lifecycle: the payment reminder opens the pay page`
