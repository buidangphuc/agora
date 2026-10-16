## ADDED Requirements

### Requirement: PDP shipping row shows real figures and no invented text

The PDP SHALL render one "Vận chuyển" row stating the free-shipping threshold of 500.000đ. For a signed-in buyer with a default address the row SHALL also state the below-threshold fee returned by `CalculateShippingFee` for that address city; for an anonymous visitor, a buyer without a default address, or when the server call fails, it SHALL state the range 20.000đ to 35.000đ. The PDP SHALL NOT render "Miễn phí vận chuyển cho đơn hàng từ 0Đ", "SPX Express (1-2 ngày)" or "Bảo hiểm thiết bị và bảo vệ người mua", and those texts SHALL be members of `FABRICATED_PDP_TEXT`.

#### Scenario: Advertised price: PDP shows the buyer's city fee
- **WHEN** a signed-in buyer whose default address city is "Hà Nội" opens a PDP and the server returns `shipping_fee = 20000` for `CalculateShippingFee("Hà Nội", 0)`
- **THEN** the shipping row contains "500.000" and "Hà Nội" and "20.000" and does not contain "35.000"
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `shows the city fee for a default address`

#### Scenario: Advertised price: anonymous visitor sees the threshold and the range
- **WHEN** an anonymous visitor opens a PDP
- **THEN** the shipping row contains "500.000", "20.000" and "35.000" and no `CalculateShippingFee` call is made
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `anonymous gets the range without a server call`

#### Scenario: Advertised price: no default address falls back to the range
- **WHEN** a signed-in buyer with no saved address opens a PDP
- **THEN** the shipping row shows the range 20.000đ to 35.000đ and no `CalculateShippingFee` call is made
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `no default address shows the range`

#### Scenario: Advertised price: a failed fee call degrades to the range
- **WHEN** `CalculateShippingFee` fails with an error for a signed-in buyer with a default address
- **THEN** the PDP still renders with status 200, the shipping row shows the range, and no fabricated fee is shown
- **VERIFIED BY**: `team-frontend/src/features/listing/ShippingInfo.test.tsx` `server failure shows the range`

#### Scenario: Advertised price: removed shipping and insurance texts are never rendered
- **WHEN** any PDP is rendered for a listing with no data behind shipping provider or insurance
- **THEN** none of "Miễn phí vận chuyển cho đơn hàng từ 0Đ", "SPX Express (1-2 ngày)" and "Bảo hiểm thiết bị và bảo vệ người mua" appears, and all three are in `FABRICATED_PDP_TEXT`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/listing/[id]/page.test.tsx` `shows no invented numbers for a listing with no reviews and no sale`

#### Scenario: Advertised price: PDP never exposes another buyer's city
- **WHEN** two different signed-in buyers with default cities "Hà Nội" and "Đà Nẵng" open the same PDP
- **THEN** each page names only that buyer's own city, and an anonymous request names no city
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: PDP never exposes another buyer's city`

#### Scenario: Advertised price: displayed shipping figures match the server rule
- **WHEN** the real `CalculateShippingFee` is called with subtotal 500000, with subtotal 499999 for "Hà Nội" and for "Cần Thơ", and with subtotal 0 for "Hà Nội" (the subtotal the PDP sends)
- **THEN** it returns free at 500000, a fee of 20000 for "Hà Nội" at 499999 and at 0, and 35000 for "Cần Thơ" at 499999, equal to `FREE_SHIPPING_THRESHOLD` and `SHIPPING_FEE_RANGE` in the PDP copy
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: displayed shipping figures match the server rule`

### Requirement: Checkout shipping and total come from the server

The checkout page SHALL obtain the shipping fee from `CalculateShippingFee(address.city, cart.subtotal)` using the server cart subtotal, SHALL pass the same server cart subtotal to the voucher preview, and SHALL NOT contain a local shipping rule (`computeShippingFee` and `src/features/order/shipping.ts` SHALL NOT exist). For a single-shop cart the shown total SHALL equal `total_amount` of the placed order. IF the shipping call fails THEN the page SHALL fail to `error.tsx` and SHALL NOT show a total.

#### Scenario: Advertised price: checkout shipping uses the server subtotal
- **WHEN** a buyer with a default address in "Hà Nội" opens checkout with a cart whose server subtotal is 300000
- **THEN** `CalculateShippingFee` is called with city "Hà Nội" and items subtotal 300000, and the summary shows the fee the server returned
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `shipping comes from CalculateShippingFee on the cart subtotal`

#### Scenario: Advertised price: free shipping at the threshold
- **WHEN** the cart subtotal is 500000 and the server answers `is_free_shipping = true`, `shipping_fee = 0`
- **THEN** the summary shows free shipping and the total equals the subtotal minus the discount
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `free shipping at the threshold`

#### Scenario: Advertised price: voucher preview uses the server cart subtotal
- **WHEN** the buyer applies voucher code "V1" on a cart whose server subtotal (flash prices included) is 400000
- **THEN** `previewVoucher` is called with items subtotal 400000 and the discount shown is the server's `discountAmount`
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `voucher preview receives the server subtotal`

#### Scenario: Advertised price: a failed shipping call does not show a made-up total
- **WHEN** `CalculateShippingFee` returns an error during checkout render
- **THEN** the page throws to the error boundary and no total or shipping fee is rendered
- **VERIFIED BY**: `team-frontend/src/app/(checkout)/checkout/(summary)/page.test.tsx` `shipping failure raises instead of inventing a fee`

#### Scenario: Advertised price: a failed shipping call returns no fee
- **WHEN** the gateway `CalculateShippingFee` call rejects or exceeds 2000 ms
- **THEN** `calculateShippingFee` returns `null` and not a 20000 fee
- **VERIFIED BY**: `team-frontend/src/lib/gateway/orders.test.ts` `calculateShippingFee returns null on error`

#### Scenario: Advertised price: shown checkout total equals the placed order total
- **WHEN** a buyer with default address city "Hà Nội" and a single-shop cart with subtotal 350000 (one flash-priced line) opens the confirm step and places the order
- **THEN** the confirm step shows shipping 20000 and total 370000, and the created order's `total_amount` is 370000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: shown checkout total equals the placed order total`

### Requirement: A price change at checkout is explained and recoverable

WHEN `checkoutAction` receives `FAILED_PRECONDITION` with the reason "price changed", the system SHALL return a failure flagged `priceChanged` with the fixed Vietnamese message, SHALL revalidate `/cart` and `/checkout`, and SHALL NOT report success or fire the `purchase` event. The checkout page SHALL then show the message and the re-priced lines and total, and the buyer SHALL be able to place the order again at the new price. Any other error SHALL keep the existing generic handling.

#### Scenario: Advertised price: action maps the price-changed refusal
- **WHEN** `createOrder` rejects with a `ConnectError` of code `FailedPrecondition` and raw message "price changed"
- **THEN** `checkoutAction` returns `{ ok: false, priceChanged: true }` with the message "Giá một số sản phẩm trong giỏ đã thay đổi. Giỏ hàng đã được cập nhật, vui lòng kiểm tra tổng tiền mới rồi đặt hàng lại." and revalidates `/cart` and `/checkout`
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `maps price changed to a refresh-worthy failure`

#### Scenario: Advertised price: another failed precondition keeps the generic error
- **WHEN** `createOrder` rejects with `FailedPrecondition` and a raw message that does not contain "price changed"
- **THEN** `checkoutAction` returns `{ ok: false }` without `priceChanged` and with the server message as today
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `other failed preconditions are not price changes`

#### Scenario: Advertised price: the form refreshes and unlocks after a price change
- **WHEN** `checkoutAction` returns `priceChanged: true` in `PlaceOrderForm`
- **THEN** the alert shows the price-changed message, `router.refresh()` is called once, the place-order button is enabled again, and no `purchase` event is tracked
- **VERIFIED BY**: `team-frontend/src/features/order/PlaceOrderForm.test.tsx` `price changed refreshes the summary and unlocks`

#### Scenario: Advertised price: the buyer sees the new total and places again
- **WHEN** a buyer carts a flash-priced listing, the campaign is switched off, and the buyer places the order from the confirm step
- **THEN** no order exists for the buyer, the page shows the Vietnamese price-changed message and a total built from the regular price, and placing the order again creates one order whose `total_amount` equals that total
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: the buyer sees the new total and places again`

#### Scenario: Advertised price: a sale that started after add is also explained
- **WHEN** a buyer's cart holds a line at the regular price and a flash campaign then starts for it, and the buyer places the order
- **THEN** the same message is shown, no order exists, and the refreshed total uses the sale price
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: a sale that started after add is also explained`

#### Scenario: Advertised price: an anonymous visitor is redirected from checkout
- **WHEN** an anonymous visitor opens `/checkout`
- **THEN** the response redirects to `/login`, and no cart, shipping fee or price-changed message is rendered
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` `Advertised price: an anonymous visitor is redirected from checkout`

#### Scenario: Advertised price: other errors from CreateOrder do not touch the cart
- **WHEN** `createOrder` rejects with `Unavailable`
- **THEN** `checkoutAction` returns `{ ok: false }` without `priceChanged` and `revalidatePath` is not called for `/cart`
- **VERIFIED BY**: `team-frontend/src/features/order/actions.test.ts` `unavailable keeps the generic error and leaves the cart`

### Requirement: The gateway routes the shipping fee quote

team-gateway SHALL forward `OrderService.CalculateShippingFee` to team-order as a read call with the caller's forwarded principal, the same way it forwards the other order reads, and SHALL return team-order's response unchanged (Clarifications Round 2).

#### Scenario: Advertised price: the gateway forwards the shipping fee quote
- **GIVEN** team-gateway with an order client that answers `CalculateShippingFee(city "Hà Nội", items_subtotal 350000)` with fee 20000, is_free false
- **WHEN** a caller sends that request to the gateway's `OrderService/CalculateShippingFee`
- **THEN** the gateway returns fee 20000 and is_free false, the order client received exactly that city and subtotal, and the call is not answered 501 Unimplemented
- **VERIFIED BY**: `team-gateway/internal/edge/order_test.go` TestOrderForwarderCalculateShippingFee

#### Scenario: Advertised price: an order-service error on the fee quote is passed through
- **GIVEN** team-gateway with an order client whose `CalculateShippingFee` fails with gRPC `UNAVAILABLE`
- **WHEN** a caller requests a shipping fee quote
- **THEN** the gateway answers with the mapped connect error `unavailable` and no fee
- **VERIFIED BY**: `team-gateway/internal/edge/order_test.go` TestOrderForwarderCalculateShippingFeeError


#### Scenario: Advertised price: the fee quote needs no buyer scope
- **GIVEN** team-gateway with an order client that answers a fee quote
- **WHEN** an anonymous caller (no token) requests `CalculateShippingFee(city "Cần Thơ", items_subtotal 100000)`
- **THEN** the gateway forwards it with the anonymous principal and returns the quote, not 401 or 403, because the quote holds only the published shipping rule and no personal data
- **VERIFIED BY**: `team-gateway/internal/edge/order_test.go` TestOrderForwarderCalculateShippingFeeAnonymous
