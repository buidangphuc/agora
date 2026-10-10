# ui-cart-checkout Specification

## Purpose
Defines the Cart & Checkout UI of `team-frontend` (`/cart`, `/checkout`, `/checkout/pay/[id]`), built only
from the `ui-components` set and modelled on the Ant Design Pro Step Form, Result and Alert templates.
Data flow, gateway calls and business rules are unchanged.

## Requirements

### Requirement: Cart is a server-rendered page grouped by shop

`/cart` SHALL render as a server component. Items SHALL be grouped by `sellerId` into one `Card` per shop.
Each group SHALL contain: a shop header (the real shop display name, falling back to "Shop #" followed by the first 6 characters of the
`sellerId` only when the name is empty, and a link to `/shop/<sellerId>`); one row per item with
`Image` (`aspect-square`, `getImageUrl()` with fallback), title, variant name, unit `PriceTag`,
`QuantityPicker` (min 1), line total and a remove control; and a voucher selector row that opens
`VoucherModal`. The page header SHALL show a `Breadcrumb` (Trang chủ > Giỏ hàng) on desktop and a "Xóa tất
cả" button. A sticky `OrderSummary` SHALL show subtotal, and a primary "Mua hàng" button linking to
`/checkout`, disabled with an explanatory `Alert` when the checkout kill-switch is off.

#### Scenario: Items from two shops render as two groups

- **WHEN** a buyer has items from seller A and seller B in the cart and opens `/cart`
- **THEN** two shop `Card`s are rendered, each with its own header and only that seller's item rows, and
  the summary subtotal equals the sum of all line totals

#### Scenario: Shop header shows the real shop name

- **WHEN** the cart has an item from a shop whose display name is "Cửa hàng Hoa Mai"
- **THEN** that shop's group header shows "Cửa hàng Hoa Mai" linking to `/shop/<sellerId>` and not "Shop #"

#### Scenario: Empty shop name falls back to the short id

- **WHEN** a shop's display name is empty
- **THEN** its group header shows "Shop #" followed by the first 6 characters of the `sellerId`

#### Scenario: The cart shows no invented shop data

- **WHEN** `/cart` is rendered
- **THEN** the shop header contains no rating, response rate, follower count or badge that is not returned by
  the gateway

#### Scenario: Cart page is not a client component

- **WHEN** the production build of `/cart` is inspected
- **THEN** `app/cart/page.tsx` has no `"use client"` directive and the client bundle for the route
  contains only the leaf islands (quantity control, clear button, voucher modal)

#### Scenario: Checkout kill-switch disables the CTA

- **WHEN** the checkout kill-switch is off and the buyer opens `/cart`
- **THEN** the "Mua hàng" button is disabled with `aria-disabled="true"` and an `Alert` explains that
  checkout is temporarily unavailable

### Requirement: Cart mutations show pending, disabled and toast feedback

Changing quantity, removing an item and clearing the cart SHALL be Server Actions returning
`{ ok, error?, data? }` and calling `revalidatePath`. While an action is pending the affected controls SHALL
be disabled with a loading indicator and the page SHALL NOT mirror cart data in client state. A success
SHALL show a success or info toast; a failure SHALL show an error toast and leave the displayed quantity at
its server value.

#### Scenario: Increasing quantity shows pending and then the new total

- **WHEN** the buyer clicks the increase button of a `QuantityPicker`
- **THEN** that picker and the remove button are disabled with `aria-busy="true"` until the action settles,
  and afterwards the quantity, line total and summary subtotal show the server values

#### Scenario: A failed quantity update is reported and not applied

- **WHEN** `updateCartItemAction` returns `ok:false` (for example insufficient stock)
- **THEN** an error toast with the reason is shown, the quantity returns to its previous server value, and
  the picker is enabled again

#### Scenario: Clearing the cart shows the empty state

- **WHEN** the buyer clicks "Xóa tất cả" and the action succeeds
- **THEN** the button was disabled and loading while pending, an info toast is shown, and the page renders
  the `Empty` state with a "Tiếp tục mua sắm" link to `/`

#### Scenario: Quantity cannot go below the minimum

- **WHEN** a cart item has quantity 1 and the buyer presses decrease
- **THEN** the decrease control is disabled and no action is called

### Requirement: Vouchers are selected through a VoucherModal

The voucher selector SHALL open a `Modal` containing a code `Input` inside a `FormItem` with an "Áp dụng"
`Button`, the sellers' available vouchers (from the existing voucher read) as selectable `Radio` cards, and
an `Empty` state when there are none. Applying SHALL call `previewVoucherAction(code, subtotal, sellerId)`;
the discount SHALL come from that response and SHALL never be computed in the browser. An invalid code SHALL
show the reason in the `FormItem` error and an error toast and SHALL leave any previously applied voucher
unchanged. An applied voucher SHALL show as a `Tag` with a remove control.

#### Scenario: Valid code applies a discount

- **WHEN** the buyer enters "SAVE10" in the `VoucherModal` and presses "Áp dụng"
- **THEN** the button shows loading while pending, the modal closes, a success toast is shown, the summary
  discount row (`data-testid="voucher-discount"`) shows the server-provided amount and the total
  (`data-testid="order-total"`) is reduced by it

#### Scenario: Invalid code is rejected with a reason

- **WHEN** the buyer enters "BOGUS-NOPE-999" and presses "Áp dụng"
- **THEN** the `FormItem` shows the reason returned by the server, an error toast is shown, no discount is
  applied and the modal stays open

#### Scenario: Voucher modal traps focus and closes with Escape

- **WHEN** the `VoucherModal` is open and the buyer presses Tab repeatedly and then Escape
- **THEN** focus stays inside the modal while open, and Escape closes it and returns focus to the voucher
  selector that opened it

#### Scenario: No available vouchers

- **WHEN** no vouchers are available and the modal is opened
- **THEN** an `Empty` block is shown and the manual code input remains usable

### Requirement: Checkout is a four-step Step Form in a distraction-free shell

`/checkout` SHALL render inside a checkout shell with a minimal header (logo link, `Stepper`) and a secure
footer, and without the consumer mega search or bottom navigation. The `Stepper` SHALL have the steps
Địa chỉ, Vận chuyển, Thanh toán, Xác nhận. The current step SHALL be read from `?step=` and the selections
from `?addr=`, `?pay=` and `?voucher=`; the step SHALL be validated on the server (unknown value -> address;
a step whose prerequisites are missing redirects to the earliest incomplete step). Each step SHALL have a
"Tiếp tục" and, except the first, a "Quay lại" action. Completed steps SHALL be navigable from the `Stepper`.
Auth redirect to `/login` and the redirect to `/cart` for an empty cart SHALL be preserved.

#### Scenario: Shell hides consumer chrome

- **WHEN** a signed-in buyer opens `/checkout`
- **THEN** the page shows the checkout header with the `Stepper` and the secure footer, and neither the
  global search field nor the mobile bottom navigation is present

#### Scenario: Step state lives in the URL

- **WHEN** the buyer proceeds from Địa chỉ to Vận chuyển and then reloads the page
- **THEN** the URL contains `step=shipping` and `addr=<id>`, and after reload the Vận chuyển step is shown
  with the same address selected

#### Scenario: Browser back returns to the previous step

- **WHEN** the buyer is on the Thanh toán step and uses the browser back button
- **THEN** the Vận chuyển step is shown with selections preserved

#### Scenario: Skipping ahead is redirected

- **WHEN** the buyer navigates directly to `/checkout?step=confirm` with no address selected
- **THEN** the page redirects to `step=address`

#### Scenario: Empty cart and signed-out user are redirected

- **WHEN** a signed-out visitor opens `/checkout`, or a signed-in buyer opens it with an empty cart
- **THEN** the visitor is redirected to `/login` or the buyer to `/cart` respectively

### Requirement: Address step uses an AddressSelectorModal

The Địa chỉ step SHALL show the selected address in a `Descriptions` card with a "Thay đổi" button opening
`AddressSelectorModal`. The modal SHALL list the buyer's addresses as `Radio` cards (recipient, phone,
address lines, "Mặc định" `Tag`), allow adding a new address through the existing `AddressModal` form, and
confirm the choice, which updates `?addr=`. With no addresses the step SHALL show an `Empty` with an "Thêm
địa chỉ" action and "Tiếp tục" SHALL be disabled.

#### Scenario: Changing the address updates the URL

- **WHEN** the buyer opens `AddressSelectorModal`, selects a different address and confirms
- **THEN** the modal closes, the address card shows the new recipient, and the URL `addr` equals the new id

#### Scenario: Adding an address from the modal

- **WHEN** the buyer adds a new address in the modal and the action succeeds
- **THEN** a success toast is shown, the new address appears in the list and is selected

#### Scenario: No saved address blocks the step

- **WHEN** the buyer has no saved addresses
- **THEN** an `Empty` with "Thêm địa chỉ" is shown and "Tiếp tục" is disabled with `aria-disabled="true"`

### Requirement: Shipping and payment steps

The Vận chuyển step SHALL show the single available option "Giao hàng tiêu chuẩn" pre-selected, with its
fee as a `PriceTag` or a "Freeship" `Tag` when the fee is 0, computed by the existing rule (subtotal >=
500000 free; Ho Chi Minh and Ha Noi 20000; otherwise 35000). The Thanh toán step SHALL render
`PaymentOptionsGrid`: one `Radio` card per payment method (COD, MoMo demo, bank demo, card demo) with title
and description, one selectable at a time, keyboard operable (arrow keys), default COD, selection stored in
`?pay=`.

#### Scenario: Free shipping above the threshold

- **WHEN** the cart subtotal is 500000 or more
- **THEN** the shipping step shows a "Freeship" `Tag` and the summary shipping row shows 0

#### Scenario: Payment method is one-of-many and keyboard operable

- **WHEN** the buyer focuses the payment grid and presses the Down arrow
- **THEN** the next method is selected, exactly one method is checked, and `?pay=` reflects it

#### Scenario: Payment grid wraps on mobile

- **WHEN** the Thanh toán step is rendered at 375px width
- **THEN** the method cards are one per row, none overflows the viewport horizontally, and each is at least
  44px tall

### Requirement: Order summary is consistent across steps

An `OrderSummary` SHALL be visible in every step (right column on desktop, bottom bar plus `Drawer` on
mobile) with rows: items subtotal, voucher discount (`data-testid="voucher-discount"`, shows "-" when none),
shipping fee, and total (`data-testid="order-total"`, brand-coloured `PriceTag`). Total SHALL equal
`max(0, subtotal - discount + shipping)`. Numbers SHALL use `formatPrice`. The summary SHALL render the same
rows in the same positions in every step so changing a selection does not shift the layout.

#### Scenario: Total reflects discount and shipping

- **WHEN** the subtotal is 300000, the applied discount is 30000 and shipping is 20000
- **THEN** the summary shows subtotal 300000, discount 30000, shipping 20000 and total 290000

#### Scenario: Mobile summary bar and drawer

- **WHEN** the buyer views any checkout step at 375px width
- **THEN** a sticky bottom bar shows the total and the primary action, and tapping "Chi tiết" opens a
  `Drawer` with the full breakdown without horizontal scroll

### Requirement: Placing an order is protected against double submit

The "Đặt hàng" button on the Xác nhận step SHALL be disabled and show `isLoading` (`aria-busy="true"`,
`aria-disabled="true"`, width preserved) from the moment it is activated until `checkoutAction` settles. A
synchronous in-flight lock SHALL make any activation while a request is in flight a no-op, including a
second click in the same tick and Enter key submission. After a successful result the button SHALL stay
disabled until navigation completes. After a failed result or a thrown error it SHALL become enabled again.
One user action SHALL result in exactly one `checkoutAction` call and one created order.

#### Scenario: Rapid double click creates one order

- **WHEN** the buyer double-clicks "Đặt hàng" within the same tick
- **THEN** `checkoutAction` is called exactly once and exactly one order exists for the buyer

#### Scenario: Button is disabled while pending

- **WHEN** "Đặt hàng" is clicked and the action has not yet resolved
- **THEN** the button is disabled, shows a spinner, has `aria-busy="true"` and the same width as before, and
  the Back link and Stepper navigation are disabled

#### Scenario: Enter key cannot bypass the guard

- **WHEN** the order is pending and the buyer presses Enter inside the confirm form
- **THEN** no second `checkoutAction` call is made

#### Scenario: Button stays locked after success

- **WHEN** `checkoutAction` returns `ok:true` and navigation to the payment or orders page is in progress
- **THEN** the button remains disabled and no further `checkoutAction` call can be made

#### Scenario: Button is re-enabled after failure

- **WHEN** `checkoutAction` returns `ok:false`
- **THEN** the button is enabled again and an error `Alert` and an error toast are shown

### Requirement: Saga failures are shown as an Alert with a recovery action

When placing an order fails (`ok:false` or a thrown error, including stock or payment-saga failures), the
Xác nhận step SHALL show an `Alert type="error"` in a fixed slot above the actions containing the server
message and recovery actions: "Thử lại" (re-enables the order button) and "Quay lại giỏ hàng" (link to
`/cart`). The cart SHALL be unchanged and the selections SHALL be preserved. When the checkout kill-switch is
off, `/checkout` SHALL render a `Result status="info"` ("Thanh toán tạm thời không khả dụng") with a
"Quay lại giỏ hàng" action instead of the wizard.

#### Scenario: Out-of-stock failure shows a recoverable error

- **WHEN** `checkoutAction` returns `ok:false` with a stock message
- **THEN** an error `Alert` shows the message with "Thử lại" and "Quay lại giỏ hàng", the Place order button
  does not move, and the cart still contains the same items

#### Scenario: Kill-switch shows an info Result

- **WHEN** the checkout kill-switch is off and the buyer opens `/checkout`
- **THEN** a `Result` with the unavailable notice and a "Quay lại giỏ hàng" button is shown and no wizard is
  rendered

### Requirement: Payment outcome is a Result page

`/checkout/pay/[id]` SHALL render the order summary (`Descriptions`: order code, total, method) and the
payment simulator for a pending transaction, with "Thanh toán thành công" and "Thanh toán thất bại" actions
(both pending-aware: disabled and loading while the action runs, mutually exclusive). After a successful
simulation it SHALL render `Result status="success"` with actions "Xem đơn hàng" (to `/account/orders`) and
"Tiếp tục mua sắm". After a failed simulation or a failed transaction it SHALL render `Result status="error"`
with "Thử lại" and "Đổi phương thức" actions. If the order was cancelled by saga compensation it SHALL render
`Result` with an `Alert` explaining that stock was released and the order cancelled, and a link to
`/account/orders`. A missing order or transaction SHALL render `Result status="404"` with a link to
`/account/orders`. Unauthenticated users SHALL be redirected to `/login`.

#### Scenario: Successful payment shows the success Result

- **WHEN** the buyer presses "Thanh toán thành công" for a pending transaction
- **THEN** both simulator buttons are disabled while pending, and afterwards a success `Result` with a link
  to the order list is shown and the order status becomes PAID

#### Scenario: Failed payment offers recovery

- **WHEN** the buyer presses "Thanh toán thất bại"
- **THEN** an error `Result` is shown with "Thử lại" and "Đổi phương thức", and an error toast is displayed

#### Scenario: Saga compensation is explained

- **WHEN** the payment step is forced to fail and the saga cancels the order
- **THEN** the page shows an `Alert` stating that stock was released and the order was cancelled

#### Scenario: Unknown order id

- **WHEN** the buyer opens `/checkout/pay/does-not-exist`
- **THEN** a `Result status="404"` with a link to `/account/orders` is rendered

### Requirement: Cart and checkout have empty, error and loading states

Each route SHALL define: an empty state (`Empty` with action) for an empty cart and for no addresses; an
error state (`Alert` or `Result` with a "Thử lại" action via the segment `error.tsx`) when the gateway read
fails; a not-found state for `/checkout/pay/[id]`; and a route `loading.tsx` with `Skeleton` blocks of the
same footprint as the loaded page.

#### Scenario: Empty cart

- **WHEN** a buyer with an empty cart opens `/cart`
- **THEN** the empty cart state (`Empty` with a "Tiếp tục mua sắm" link) is displayed and no summary or
  "Mua hàng" button is rendered

#### Scenario: Gateway failure on the cart

- **WHEN** the cart read throws
- **THEN** the segment error UI shows an `Alert` with "Thử lại" which re-renders the route

#### Scenario: Loading skeleton matches the page footprint

- **WHEN** `/cart` is streaming
- **THEN** `loading.tsx` renders shop-card, three item-row and summary `Skeleton`s, and on load completion
  the page's cumulative layout shift score is 0

### Requirement: Images and layout cause no layout shift

Item thumbnails SHALL use `Image` with a fixed square aspect and `getImageUrl()` fallback; thumbnails below
the first shop group SHALL use `loading="lazy"`. Dynamic values (discount, shipping, totals, voucher state)
SHALL change text inside fixed rows and SHALL NOT insert or remove rows.

#### Scenario: Applying a voucher does not shift the layout

- **WHEN** a voucher is applied in checkout
- **THEN** the vertical position of the "Đặt hàng"/"Tiếp tục" button and of the total row is unchanged

#### Scenario: Below-the-fold thumbnails are lazy

- **WHEN** the cart has three shop groups
- **THEN** images in the second and third groups carry `loading="lazy"`

### Requirement: Layout is responsive at 375px and desktop

At 375px width the cart and checkout SHALL render in one column with no horizontal scrolling, a compact
`Stepper`, a sticky bottom action bar, `Modal`s as full-height scrolling sheets and tap targets of at least
44px. At desktop (>= 1024px) they SHALL render two columns with the order summary sticky in the right
column and a horizontal labelled `Stepper`.

#### Scenario: Mobile cart fits the viewport

- **WHEN** `/cart` with items from two shops is opened at 375px width
- **THEN** `document.documentElement.scrollWidth` does not exceed 375 and the quantity picker and "Mua hàng"
  button are visible without horizontal scroll

#### Scenario: Desktop shows a sticky summary

- **WHEN** `/checkout` is opened at 1280px width and the page is scrolled
- **THEN** the order summary stays visible in the right column

### Requirement: Cart and checkout use the design tokens and type scale

All text SHALL use the 12/14/16/20/24px type scale; no arbitrary values (`text-[10px]`, `p-[7px]`) and no
raw hex colours SHALL appear in the files of this change. Brand colour SHALL be used only for primary CTAs
(Mua hàng, Tiếp tục, Đặt hàng), `PriceTag` totals and badges.

#### Scenario: Token lint is clean

- **WHEN** the token lint from `ui-foundation` runs over `features/cart`, `features/order` checkout files,
  `features/payment` and `app/cart`, `app/checkout`
- **THEN** it reports no arbitrary values, raw hex colours or font sizes outside the scale

### Requirement: Tracking hooks are unchanged

The ecommerce tracking events SHALL still fire with the same names and payloads: `begin_checkout` once when
checkout is entered (not on every step change), `apply_promotion` on each voucher preview with `coupon`,
`value` and `properties.valid`, and `purchase` once after a successful order with `transactionId`,
`currency`, `value`, `coupon`, `shippingTier`, `paymentType` and `items`. `AnalyticsProvider`, `TrackLink`,
`TrackImpression` and `data-*` attributes SHALL be unchanged.

#### Scenario: begin_checkout fires once per entry

- **WHEN** the buyer opens `/checkout` and moves through all four steps
- **THEN** exactly one `begin_checkout` event is sent, with the cart items and value

#### Scenario: purchase fires once

- **WHEN** the buyer places an order successfully, including after a double click
- **THEN** exactly one `purchase` event is sent with the created order id as `transactionId`

#### Scenario: apply_promotion still fires for valid and invalid codes

- **WHEN** the buyer previews a valid code and then an invalid code
- **THEN** two `apply_promotion` events are sent, with `properties.valid` equal to `"true"` and `"false"`
  respectively
