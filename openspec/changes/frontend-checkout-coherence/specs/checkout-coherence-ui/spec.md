## ADDED Requirements

### Requirement: Checkout shows one group per shop with server figures

The checkout page SHALL obtain every figure from `OrderService.PreviewCheckout` (request: `address_id`, `item_ids`, shop vouchers, platform voucher) and SHALL render one group per `CheckoutGroupQuote` showing items subtotal, shipping (or "Freeship" when `is_free_shipping`), shop discount, platform-voucher share, group total and its delivery date, plus a summary whose subtotal, shipping, discount and total are the response totals. The page SHALL NOT sum, prorate or apply a free-shipping threshold itself. IF `PreviewCheckout` fails THEN the page SHALL fail to `error.tsx` and SHALL NOT show a total; IF it answers FAILED_PRECONDITION "cart is empty" THEN the page SHALL redirect to `/cart`.

#### Scenario: Checkout coherence: two shops show two groups with their own shipping
- **WHEN** a buyer with an address in "Đà Nẵng" opens checkout for items of shop A (300000) and shop B (300000) and the server answers two groups each with `items_subtotal = 300000`, `shipping_fee = 35000`, `is_free_shipping = false`
- **THEN** two groups render, each showing 300.000 and 35.000 (no "Freeship"), and the summary shipping row shows the response `shipping_fee` 70000 and the total row shows the response `total_amount` 670000
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `two shops show two groups with their own shipping`

#### Scenario: Checkout coherence: free shipping is decided per group
- **WHEN** the server answers one group with `items_subtotal = 500000`, `is_free_shipping = true` and another with `items_subtotal = 300000`, `shipping_fee = 35000`
- **THEN** the first group shows "Freeship" and the second shows 35.000, and the summary shipping equals the response value 35000
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `free shipping is decided per group`

#### Scenario: Checkout coherence: the frontend adds nothing up
- **WHEN** the server answers group totals whose sum differs from the response `total_amount` (a deliberately inconsistent stub)
- **THEN** the summary total row shows the response `total_amount` unchanged, proving no client arithmetic is applied
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `summary shows the response totals unchanged`

#### Scenario: Checkout coherence: a failed preview shows no total
- **WHEN** `PreviewCheckout` fails with UNAVAILABLE
- **THEN** the page throws to `error.tsx` and no total, shipping fee or group is rendered
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `a failed preview raises and shows no total`

#### Scenario: Checkout coherence: an empty preview selection returns to the cart
- **WHEN** `PreviewCheckout` fails with FAILED_PRECONDITION "cart is empty"
- **THEN** the page redirects to `/cart`
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `cart is empty redirects to the cart`

#### Scenario: Checkout coherence: the shown totals equal the placed orders
- **WHEN** a buyer with items from two shops opens checkout, reads the summary total and places the order
- **THEN** two orders exist and the sum of their `total_amount` equals the summary total shown before placing
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: the shown totals equal the placed orders`

### Requirement: Vouchers are entered per scope and only valid ones are placed

The checkout payment step SHALL offer one shop-voucher field per group and one platform-voucher field, URL state `sv=<sellerId>:<CODE>` (at most one per seller) and `pv=<CODE>`. The preview request SHALL carry them as `shop_vouchers` and `platform_voucher_code`; for each `VoucherQuote` with `valid = false` the page SHALL show its `reason` next to its field. The confirm step and `CreateOrder` SHALL carry only codes whose quote is valid, and the frontend SHALL NOT send the legacy `voucher_code`. A shop code placed in the wrong group or the platform slot (or the reverse) is judged by the server and shown with its reason.

#### Scenario: Checkout coherence: shop and platform vouchers go to their own slots
- **WHEN** the URL has `sv=<shopA>:SHOPA10` and `pv=PLAT5`
- **THEN** the `PreviewCheckout` request has `shop_vouchers = [{seller_id: shopA, code: SHOPA10}]`, `platform_voucher_code = PLAT5`, and group A shows its shop discount and its platform share while group B shows a platform share only
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `vouchers are sent in their own slots`

#### Scenario: Checkout coherence: an invalid voucher shows its reason and no discount
- **WHEN** the preview answers `valid = false`, `reason = "voucher belongs to another shop"` for `sv=<shopB>:SHOPA10`
- **THEN** the reason text appears next to group B's field, group B's discount shows "-" and its total equals subtotal plus shipping
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `invalid voucher shows its reason`

#### Scenario: Checkout coherence: only valid vouchers are placed
- **WHEN** the buyer confirms with one valid shop voucher and one invalid platform voucher
- **THEN** `checkoutAction` is called with that shop voucher and an empty platform voucher, and with no legacy `voucher_code`
- **VERIFIED BY**: `team-frontend/src/features/order/PlaceOrderForm.test.tsx` `sends only valid vouchers in scoped slots`

#### Scenario: Checkout coherence: another shop's voucher is refused for a group
- **WHEN** a buyer who is not the owner of shop A's voucher scope applies shop A's voucher code to shop B's group at checkout (forbidden scope)
- **THEN** group B shows the server's refusal reason, its discount stays "-", and the summary total equals the sum of the group totals without that voucher
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: another shop's voucher is refused for a group`

#### Scenario: Checkout coherence: two shop vouchers and a platform voucher place discounted orders
- **WHEN** a buyer with items of two shops applies shop A's voucher, shop B's voucher and one platform voucher and places the order
- **THEN** each placed order's `discount_amount` equals its group's shop discount plus platform share shown at confirm, and the summary discount equals their sum
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: two shop vouchers and a platform voucher place discounted orders`

### Requirement: Delivery date comes from the server

The PDP SHALL show "Nhận hàng dự kiến dd/mm" for a signed-in buyer with a default address from `estimated_delivery_at` of `CalculateShippingFee(city, 0)`, and each checkout group SHALL show "Nhận hàng dự kiến dd/mm" from its `estimated_delivery_at`, formatted in Asia/Ho_Chi_Minh by one helper. The frontend SHALL NOT compute "today plus n days". IF the buyer is anonymous, has no default address, `shipping_quoted` is false, the timestamp is unset or the call fails THEN no date and no placeholder SHALL be rendered and the page SHALL still render.

#### Scenario: Checkout coherence: the PDP shows the server delivery date
- **WHEN** a signed-in buyer with a default address opens a PDP and the server answers `estimated_delivery_at = 2026-10-18T10:00:00+07:00`
- **THEN** the shipping row contains "Nhận hàng dự kiến 18/10"
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `shows the server delivery date`

#### Scenario: Checkout coherence: the PDP shows no date without an address
- **WHEN** an anonymous visitor, or a signed-in buyer without a default address, opens a PDP
- **THEN** the row contains no "Nhận hàng dự kiến" and no delivery call is made
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `no delivery date without an address`

#### Scenario: Checkout coherence: a failed or unset delivery date is not invented
- **WHEN** `CalculateShippingFee` fails, or answers without `estimated_delivery_at`
- **THEN** the PDP renders with status 200 and the row shows no "Nhận hàng dự kiến"
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `failed or unset delivery date shows nothing`

#### Scenario: Checkout coherence: the date is formatted in Ho Chi Minh time
- **WHEN** the timestamp is `2026-10-17T18:30:00Z` (01:30 on the 18th in Asia/Ho_Chi_Minh), and one at `2026-12-31T16:59:59Z`
- **THEN** the helper returns "18/10" and "31/12" regardless of the process time zone
- **VERIFIED BY**: `team-frontend/src/features/order/deliveryDate.test.ts` `formats dd/mm in Asia/Ho_Chi_Minh`

#### Scenario: Checkout coherence: each checkout group shows its own date
- **WHEN** the preview answers two groups with different `estimated_delivery_at` and `shipping_quoted = true`
- **THEN** each group shows its own "Nhận hàng dự kiến dd/mm"; with `shipping_quoted = false` no group shows a date
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `each group shows its own delivery date`

#### Scenario: Checkout coherence: the delivery date matches the mock rule end to end
- **WHEN** a buyer with a default address in "Hà Nội" opens a PDP and checkout, and another buyer with city "Cần Thơ" does the same
- **THEN** the Hà Nội pages show today + 2 days and the Cần Thơ pages today + 4 days as dd/mm in Asia/Ho_Chi_Minh, and an anonymous PDP shows no date
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: the delivery date matches the mock rule end to end`

### Requirement: An order is placed only at the address the buyer saw

The confirm step SHALL display `PreviewCheckout.shipping_address` and `PlaceOrderForm` SHALL send it as `confirmed_address` together with `address_id`. `checkoutAction` SHALL map FAILED_PRECONDITION "address changed" to `addressChanged: true` with a fixed message, FAILED_PRECONDITION "address not found" to a fixed message with a link to the address step, and UNAVAILABLE to a fixed retry message; `PlaceOrderForm` SHALL on `addressChanged` release the lock and call `router.refresh()`. All other errors SHALL keep the generic handling. Address fields SHALL NOT appear in the URL, analytics events or logs.

#### Scenario: Checkout coherence: the confirmed address is sent with the order
- **WHEN** the buyer presses "Đặt hàng" with the preview address shown at confirm
- **THEN** `checkoutAction` receives `addressId` and `confirmedAddress` equal to `shipping_address`
- **VERIFIED BY**: `team-frontend/src/features/order/PlaceOrderForm.test.tsx` `sends the confirmed address`

#### Scenario: Checkout coherence: an edited address is refused and refreshed
- **WHEN** `CreateOrder` fails with FAILED_PRECONDITION "address changed"
- **THEN** the action returns `addressChanged: true` with the fixed message, no `purchase` event is tracked, the form is unlocked and `router.refresh()` is called once
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `maps address changed`

#### Scenario: Checkout coherence: a missing address is explained
- **WHEN** `CreateOrder` fails with FAILED_PRECONDITION "address not found" (including a buyer who has addresses but no default and sent an empty `address_id`; the server never falls back to the first address)
- **THEN** the action returns the fixed message and the form offers a link to the address step; no other FAILED_PRECONDITION reason is mapped to it
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `maps address not found and leaves other preconditions generic`

#### Scenario: Checkout coherence: an identity outage asks the buyer to retry
- **WHEN** `CreateOrder` fails with UNAVAILABLE
- **THEN** the action returns the fixed retry message, the form unlocks and the Idempotency-Key is kept for the retry
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `maps unavailable to a retry message`

#### Scenario: Checkout coherence: an anonymous visitor cannot reach checkout or pay pages
- **WHEN** an anonymous visitor opens `/checkout?step=confirm` or `/checkout/pay/<id>`
- **THEN** the response redirects to `/login` and no `PreviewCheckout`, `CreateOrder` or `CreatePayment` call is made
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `guest is redirected to login without any call`
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `guest is redirected to login before any payment call`

#### Scenario: Checkout coherence: an edit after confirm shows the new address
- **WHEN** a buyer reaches confirm, the address is edited through the address API, and the buyer presses "Đặt hàng"
- **THEN** no order is created, the page shows the edited address and a message that the address changed, and a second press places the order at the edited address
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: an edit after confirm shows the new address`

### Requirement: Every online-pay order of a checkout can be paid

After placing with an online method the frontend SHALL send the buyer to `/checkout/pay/<first order id>` without creating a payment itself. The pay page SHALL call `CreatePayment(order.id, order.payment_method)` when no transaction exists and the order is PENDING with an online method; a COD, non-PENDING or not-owned order without a transaction SHALL still answer 404. After a successful payment the page SHALL offer the next unpaid order of the same `checkout_id` (PENDING, online method, lowest `seller_id`) until none is left. A COD checkout SHALL create no transaction.

#### Scenario: Checkout coherence: placing an online order goes to the first pay page
- **WHEN** `CreateOrder` returns two orders for method MOCK_MOMO
- **THEN** `checkoutAction` returns `paymentUrl = /checkout/pay/<orders[0].id>`, calls `createPayment` zero times, and COD returns `/account/orders?success=1`
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `online checkout returns the first pay page without creating a payment`

#### Scenario: Checkout coherence: the pay page creates a missing transaction
- **WHEN** a buyer opens `/checkout/pay/<id>` for their PENDING MOCK_BANK order and `getPayment` finds no transaction
- **THEN** `createPayment(id, MOCK_BANK)` is called once and the page renders the amount and method with status 200
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `creates a missing transaction for a pending online order`

#### Scenario: Checkout coherence: the pay page does not create a payment where none is due
- **WHEN** the order is COD, or CANCELLED, or PAID, and has no transaction
- **THEN** `createPayment` is not called and the page answers 404 (not-found)
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `no transaction is created for COD or closed orders`

#### Scenario: Checkout coherence: another buyer's order is never paid
- **WHEN** `GetOrder` answers PermissionDenied or NotFound for the order id
- **THEN** the pay page answers 404 and neither `getPayment` nor `createPayment` is called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `an order of another buyer is not found and not paid`

#### Scenario: Checkout coherence: the next unpaid order of the checkout is offered
- **WHEN** the buyer pays the first of three orders of one `checkout_id` (the second PENDING online, the third already PAID, one other order of another checkout PENDING)
- **THEN** the success state shows a link "Thanh toán đơn tiếp theo" to the second order's pay page and no link to the third or to the other checkout's order; on the last unpaid order no such link shows
- **VERIFIED BY**: `team-frontend/src/features/payment/PaymentSimulator.test.tsx` `offers the next unpaid order of the checkout`

#### Scenario: Checkout coherence: a failed lookup of the next order does not break the pay page
- **WHEN** `ListBuyerOrders` fails while rendering the pay page of an order that has a `checkout_id`
- **THEN** the page renders with status 200 and no next-order link
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/pay/[id]/page.test.tsx` `next order lookup failure degrades to no link`

#### Scenario: Checkout coherence: both orders of a two-shop checkout are payable
- **WHEN** a buyer places a two-shop MOCK_MOMO checkout, pays the first order on its pay page and follows "Thanh toán đơn tiếp theo"
- **THEN** both orders reach status PAID, each has exactly one payment transaction, and a COD two-shop checkout creates no transaction
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: both orders of a two-shop checkout are payable`

#### Scenario: Checkout coherence: pay now works for an order without a transaction
- **WHEN** a buyer clicks "Thanh toán ngay" on `/account/orders` for a PENDING online order that has no transaction
- **THEN** the pay page shows the amount and the payment simulator (status 200, not a 404)
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/checkout_coherence_ui.feature` `Checkout coherence: pay now works for an order without a transaction`

### Requirement: The items selection is carried through checkout

`checkoutParams` SHALL parse `items` (comma-separated cart item ids, empty = whole cart), `checkoutHref` SHALL keep it across every step and every link built from the state, and the checkout page SHALL send it as `item_ids` to `PreviewCheckout` and the place-order action SHALL send it to `CreateOrder`. Lines of the cart not in `items` SHALL NOT be shown in the checkout groups.

#### Scenario: Checkout coherence: items survive every checkout step
- **WHEN** the URL has `items=i1,i2` and the buyer moves through address, shipping, payment and confirm
- **THEN** every generated href contains `items=i1%2Ci2` and the place-order action receives `itemIds = [i1, i2]`
- **VERIFIED BY**: `team-frontend/src/features/order/checkoutParams.test.ts` `items are kept by checkoutHref`

#### Scenario: Checkout coherence: only the selected lines are quoted and listed
- **WHEN** the cart has 3 lines and the URL has `items=<one id>`
- **THEN** `PreviewCheckout` receives `item_ids = [<that id>]` and the confirm list shows only that line
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `only the selected lines are quoted and listed`

#### Scenario: Checkout coherence: no items param quotes the whole cart
- **WHEN** the URL has no `items`
- **THEN** `PreviewCheckout` receives an empty `item_ids`
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `no items param quotes the whole cart`

### Requirement: Checkout keeps no local shipping, voucher or total rule

The checkout path SHALL NOT call `calculateShippingFee` or `previewVoucher` and SHALL NOT use `orderTotal` to produce a shown figure; the legacy `voucher` search param SHALL NOT be sent to the server.

#### Scenario: Checkout coherence: checkout calls only the preview for figures
- **WHEN** the checkout page renders at every step for an address and a voucher
- **THEN** `previewCheckout` is called and `calculateShippingFee` and `previewVoucher` are not called
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `figures come from the preview only`

#### Scenario: Checkout coherence: a legacy voucher link is ignored
- **WHEN** the URL has `voucher=OLD` and no `sv`/`pv`
- **THEN** the preview request has no voucher and the page shows no voucher error
- **VERIFIED BY**: `team-frontend/src/features/order/checkoutParams.test.ts` `legacy voucher param is ignored`
