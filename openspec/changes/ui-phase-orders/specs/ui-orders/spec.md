## Purpose

Defines the buyer order screens of `team-frontend`: the order list (`/account/orders`) and the order detail
(`/account/orders/[id]`), built from the `ui-components` set following the Ant Design Pro List and Advanced
Profile templates, with the order timeline, status badge and return (RMA) flow.

## ADDED Requirements

### Requirement: The order list is a server-rendered Basic List with status tabs and pagination in the URL

`/account/orders` SHALL be a server component. The list filter and page SHALL live in `searchParams`
(`?status=all|pending|paid|shipped|completed|cancelled&page=<n>`, default `all` and `1`), 10 orders per page.
The page SHALL call `listBuyerOrders()` once, filter by status and slice on the server. An unknown `status`
SHALL behave as `all`; a `page` beyond the last page SHALL clamp to the last page. Only the tab bar
(`OrderStatusTabs`) is a client island; `Pagination` renders as links. Each tab SHALL show its order count.
Unauthenticated users SHALL be redirected to `/login`.

#### Scenario: Selecting a status tab updates the URL and the list

- **WHEN** a buyer with orders in several statuses opens `/account/orders` and selects the "Đã giao" (completed)
  tab
- **THEN** the URL becomes `/account/orders?status=completed`, only completed orders are listed, and `page`
  is reset to 1

#### Scenario: The list state survives reload and back navigation

- **WHEN** a buyer opens `/account/orders?status=paid&page=2`, then reloads, or navigates away and back
- **THEN** the same tab is active and the same page of orders is shown

#### Scenario: Pagination links page through the filtered list

- **WHEN** a buyer has 23 orders and opens `/account/orders`
- **THEN** 10 orders are shown with a `Pagination` of 3 pages, and following the "3" link shows the last 3
  orders at `?page=3`

#### Scenario: An invalid query falls back safely

- **WHEN** a buyer opens `/account/orders?status=bogus&page=99`
- **THEN** the "all" tab is active and the last page of orders is shown, with no error

#### Scenario: A guest is redirected

- **WHEN** an unauthenticated visitor opens `/account/orders`
- **THEN** they are redirected to `/login`

### Requirement: Order rows show status, items and actions with pending feedback

Each order row SHALL be a `Card` showing the `OrderStatusBadge`, item rows (`Image` thumbnail, title, variant,
quantity, `PriceTag`), the order total and actions: "Xem chi tiết" (link to `/account/orders/[id]`), "Mua
lại", "Hủy đơn" (only where cancel is offered today) and "Đánh giá" (opens the existing `ReviewModal`).
"Mua lại" and cancel SHALL be Server Actions returning `{ ok, error?, data? }` that call `revalidatePath`; each
SHALL show a pending state (`isLoading`, width preserved), disable the row's actions while pending, and show a
success or error toast. Cancel confirmation SHALL be a `Modal`, not `window.confirm`. The client SHALL NOT keep a
local copy of the orders array.

#### Scenario: Reordering shows pending then success

- **WHEN** a buyer clicks "Mua lại" on an order
- **THEN** the button shows a spinner with its width unchanged and is disabled, and on success a success toast
  appears and the buyer lands on `/cart`

#### Scenario: Reorder failure is reported and recoverable

- **WHEN** the reorder action returns `{ ok: false, error }`
- **THEN** an error toast shows the error, the button becomes enabled again, and the buyer stays on the list

#### Scenario: Cancelling an order uses a confirmation Modal

- **WHEN** a buyer clicks "Hủy đơn" on a cancellable order and confirms in the `Modal`
- **THEN** the confirm button shows a pending state and the Modal cannot be dismissed, then a success toast
  shows, the Modal closes and the order's badge reads "Đã hủy" from the revalidated server data

#### Scenario: Cancel failure keeps the Modal open

- **WHEN** the cancel action returns `{ ok: false, error }`
- **THEN** an error toast shows and the Modal stays open with the confirm button enabled

### Requirement: Order screens show only real data

Order rows SHALL show the real shop display name for the order's seller (from the `shop-display-name`
contract), falling back to "Shop #" followed by the first 6 characters of the seller id only when the name is
empty. Order screens SHALL NOT render invented values (delivery estimates, carrier names, shop stats, ratings
or badges not present in the order, shipment or saga responses); a block with no data SHALL be hidden or show
`Empty`.

#### Scenario: Order row shows the real shop name

- **WHEN** an order's seller has the display name "Cửa hàng Hoa Mai"
- **THEN** the order row shows "Cửa hàng Hoa Mai" and not "Shop #"

#### Scenario: Empty shop name falls back

- **WHEN** the seller's display name is empty
- **THEN** the order row shows "Shop #" followed by the first 6 characters of the seller id

### Requirement: OrderStatusBadge is the single status presentation

`OrderStatusBadge` SHALL be a `Tag`-based component used wherever an order or return status is shown
(list rows, detail header, return section). It SHALL map `OrderStatus` (PENDING, PAID, SHIPPED, COMPLETED,
CANCELLED) and `ReturnStatus` (PENDING, APPROVED, REJECTED, REFUNDED) to semantic tones from design tokens
(warning, info, success, danger, neutral), render the status text, use no emoji, no arbitrary font sizes
(12px), and not use the brand colour. An unknown status SHALL render a neutral tag with its text.

#### Scenario: Statuses map to tones

- **WHEN** `OrderStatusBadge` is rendered for PENDING, SHIPPED, COMPLETED and CANCELLED orders
- **THEN** it renders warning, info, success and neutral tones respectively with the given label text

#### Scenario: Return statuses use the same component

- **WHEN** the return section shows a REFUNDED return and a REJECTED return
- **THEN** both use `OrderStatusBadge` with success and danger tones, and `data-testid="return-status"` is kept

#### Scenario: An unknown status does not break the row

- **WHEN** an order arrives with an unrecognised status
- **THEN** a neutral badge with its `statusText` is rendered and the page does not throw
- **VERIFIED BY**: Vitest `team-frontend/src/features/order/OrderStatusBadge.test.tsx` › OrderStatusBadge › renders an unknown status as a neutral tag with its text. Not verifiable end to end: the gateway never returns an unrecognised order status, so the state cannot be seeded through the edge.

### Requirement: Empty, error and loading states of the list

When the selected tab has no orders, the list SHALL render `Empty` with a recovery action: for `all` a
"Mua sắm ngay" link to `/`; for other tabs a "Xem tất cả đơn hàng" link to `?status=all`. When loading the
list fails, an `Alert type="error"` with a retry action SHALL be shown. The route SHALL provide a
`loading.tsx` whose `Skeleton` matches the tab bar and three order cards. The list SHALL NOT show an empty
state when the load failed.

#### Scenario: A buyer with no orders sees Empty with a shopping action

- **WHEN** a buyer with no orders opens `/account/orders`
- **THEN** an `Empty` block with "Bạn chưa có đơn hàng nào" and a "Mua sắm ngay" link to `/` is shown, and the
  tab counts read 0

#### Scenario: An empty tab offers a way back

- **WHEN** a buyer with no cancelled orders opens `?status=cancelled`
- **THEN** an `Empty` block for that tab is shown with a link to `?status=all`

#### Scenario: A failed load shows an Alert with retry

- **WHEN** the order list request fails
- **THEN** an `Alert` with an error message and a "Thử lại" action is shown instead of the empty state

#### Scenario: Loading does not shift layout

- **WHEN** the list route is loading
- **THEN** `loading.tsx` renders a `Skeleton` of the same height as the tab bar and the order cards, and the
  content replaces it without moving the footer (CLS = 0)

### Requirement: The order detail follows the Advanced Profile anatomy

`/account/orders/[id]` SHALL be a server component composed of: a header (`Breadcrumb`, "Order #<id>" title,
`OrderStatusBadge`, total `PriceTag`, action buttons); a progress band with a 5-step `Stepper`; `Descriptions`
for recipient, phone, address, payment method and voucher; an items `Table`; an amounts `Descriptions`
(subtotal, shipping, discount, total); and `Tabs` ("Hành trình", "Trả hàng / Hoàn tiền") whose active tab is held
in `?tab=` (default `timeline`). Header actions: "Mua lại" (primary), "Yêu cầu trả hàng" (COMPLETED orders)
and "Hủy đơn" (cancellable orders). The cancel flow SHALL call `cancelOrderAction` (it SHALL NOT only set local
state and reload). When the order is CANCELLED the `Stepper` SHALL be replaced by an `Alert` "Đơn hàng đã hủy".

#### Scenario: The detail shows header, progress, descriptions, items and tabs

- **WHEN** a buyer opens the detail of their SHIPPED order
- **THEN** the header shows the breadcrumb, order id, a "shipped" `OrderStatusBadge` and the total; the
  `Stepper` marks "Đang vận chuyển" with `aria-current="step"`; recipient and payment `Descriptions` and the
  items `Table` are visible; and the "Hành trình" tab is active

#### Scenario: The active tab is held in the URL

- **WHEN** a buyer selects the "Trả hàng / Hoàn tiền" tab
- **THEN** the URL gains `?tab=returns`, and reloading that URL shows the same tab

#### Scenario: A cancelled order shows an Alert instead of the Stepper

- **WHEN** a buyer opens a CANCELLED order
- **THEN** an `Alert` "Đơn hàng đã hủy" is shown, no `Stepper` is rendered, and "Hủy đơn" is not offered

#### Scenario: Cancelling from the detail really cancels

- **WHEN** a buyer confirms "Hủy đơn" in the detail `Modal`
- **THEN** `cancelOrderAction` is called once with the order id, a success toast shows, and after revalidation
  the header badge reads "Đã hủy"

### Requirement: OrderTimeline renders checkpoints and the saga failure checkpoint

`OrderTimeline` SHALL be built on `Timeline` (vertical). Source precedence is: shipment checkpoints (newest
first, the first marked current), else saga steps, else `Empty` ("Chưa có thông tin vận chuyển cho đơn hàng
này."). A shipment header SHALL show carrier, tracking code and shipment status. In the saga view a step with
status SUCCESS SHALL render as success, PENDING as pending, and FAILED or COMPENSATED as the **failure
checkpoint**: an error item showing the step name, detail and timestamp with `data-testid="timeline-failure"`,
plus an `Alert type="error"` above the timeline with the recovery action "Mua lại". The existing testids
`order-timeline`, `timeline-checkpoint`, `timeline-saga`, `timeline-saga-step`, `timeline-empty` SHALL be
kept. The component SHALL be server-compatible (no `"use client"`) and be wrapped in `Suspense` with a
`Skeleton` fallback of the same footprint.

#### Scenario: Shipment checkpoints are listed newest first

- **WHEN** an order has a shipment with three checkpoints
- **THEN** three `timeline-checkpoint` items are shown, the newest first and marked current, with carrier and
  tracking code in the header
- **VERIFIED BY**: Vitest `team-frontend/src/features/order/OrderTimeline.test.tsx` › OrderTimeline › lists checkpoints newest first and marks the newest as current. Not verifiable end to end: CreateShipment yields exactly one checkpoint and no RPC adds more, so a shipment with several checkpoints cannot be seeded through the edge (the test uses two checkpoints, which exercises the same ordering).

#### Scenario: A failed saga step is surfaced as the failure checkpoint

- **WHEN** an order has no shipment and its saga steps are order SUCCESS, stock SUCCESS, payment FAILED,
  stock-release COMPENSATED
- **THEN** the failed and compensated steps render as error items, at least one has
  `data-testid="timeline-failure"` showing its detail, and an error `Alert` with a "Mua lại" action is shown

#### Scenario: A pending saga step is distinguished from failure

- **WHEN** the saga steps are order SUCCESS and payment PENDING
- **THEN** the payment step renders as pending, no `timeline-failure` item exists, and no error `Alert` is shown

#### Scenario: No tracking information shows Empty

- **WHEN** an order has neither shipment checkpoints nor saga steps
- **THEN** `timeline-empty` renders an `Empty` with the configured text
- **VERIFIED BY**: Vitest `team-frontend/src/features/order/OrderTimeline.test.tsx` › OrderTimeline › shows an empty state when there is neither shipment nor saga. Not verifiable end to end: every order created through the edge has saga steps, so an order with neither a shipment nor saga steps cannot be seeded.

### Requirement: Returns are requested in a Modal with validation and feedback

`ReturnRequestSection` SHALL show the existing return as `Descriptions` (reason, refund amount) with an
`OrderStatusBadge`, or an `Empty` ("Chưa có yêu cầu trả hàng") when none exists. The return request form SHALL
open in a `Modal` from the header button "Yêu cầu trả hàng", with `FormItem` fields: reason (`Select`,
required) and refund amount (`Input` number, greater than 0 and at most the order total, default the order
total). Submitting SHALL call `createReturnRequestAction` and, while pending, show `isLoading` on the submit
button, disable the fields and prevent dismissing the Modal. Success SHALL close the Modal, show a success
toast and refresh the section from revalidated data; failure SHALL keep the Modal open and show an error toast.
The buyer's return section SHALL NOT offer any refund control: refunding is a seller action (the seller
order page's returns tab, "Hoàn tiền" with a confirm `Modal`, `data-testid="return-refund"` in `SellerReturns`),
so the buyer only sees the resulting return status. The existing
testids `return-section`, `return-reason`, `return-amount`, `return-submit`, `return-status` SHALL be kept.

#### Scenario: Submitting a valid return

- **WHEN** a buyer opens "Yêu cầu trả hàng", chooses a reason, keeps the default amount and submits
- **THEN** the submit button shows a pending state, then the Modal closes, a success toast shows, and the
  section shows the return with a "pending" `OrderStatusBadge`

#### Scenario: Validation blocks an invalid form

- **WHEN** a buyer submits with no reason, or with an amount greater than the order total
- **THEN** the corresponding `FormItem` shows an inline error, no action is called, and the Modal stays open

#### Scenario: A failed submission keeps the Modal open

- **WHEN** `createReturnRequestAction` returns `{ ok: false, error }`
- **THEN** an error toast shows the error, the Modal stays open and the submit button is enabled again

#### Scenario: The buyer sees the return status without a refund button

- **WHEN** a buyer opens an order whose return is APPROVED or REFUNDED
- **THEN** the return section shows the matching `OrderStatusBadge` and no refund button
  (`return-refund` is absent); the refund itself is performed by the seller

#### Scenario: The return is not offered for non-eligible orders

- **WHEN** a buyer opens a PENDING order
- **THEN** the "Yêu cầu trả hàng" button is not shown

### Requirement: Other users' and missing orders render Result pages, not data

The detail page SHALL resolve the order with a discriminated result (`ok`, `forbidden`, `not_found`, `error`).
For `forbidden` it SHALL render `Result status="403"` ("Bạn không có quyền xem đơn hàng này") with the action
"Về đơn hàng của tôi" linking to `/account/orders`, and SHALL NOT fetch or render the shipment, saga, items or
any order field. For `not_found` it SHALL render `Result status="404"` with the same recovery action. For
`error` it SHALL render an `Alert` with a retry action. Unauthenticated users SHALL be redirected to `/login`.

#### Scenario: An order of another user shows 403

- **WHEN** buyer A opens `/account/orders/<id>` for an order owned by buyer B
- **THEN** a 403 `Result` is shown with a link to `/account/orders`, and no recipient, item or amount of that
  order appears in the page

#### Scenario: A missing order shows 404

- **WHEN** a buyer opens `/account/orders/does-not-exist`
- **THEN** a 404 `Result` is shown with a link to `/account/orders`

#### Scenario: A transport failure offers retry

- **WHEN** the order request fails with a non-permission, non-not-found error
- **THEN** an `Alert` with a "Thử lại" action is shown, and not a 404

### Requirement: Order screens never shift layout

Both routes SHALL provide `loading.tsx` with `Skeleton` blocks matching their footprint. Async blocks on the
detail (timeline, returns) SHALL be wrapped in `Suspense` with a same-height `Skeleton`. Item thumbnails SHALL
use `Image` with a fixed 1:1 box and the `getImageUrl()` fallback; images in the first order card or the first
screen are eager, the rest below the fold SHALL be `loading="lazy"`. Pending buttons SHALL preserve their width.

#### Scenario: Thumbnails reserve their box

- **WHEN** order thumbnails load slowly or fail
- **THEN** each keeps a fixed 1:1 box (fallback placeholder on failure) and surrounding text does not move

#### Scenario: Below-the-fold images are lazy

- **WHEN** the list shows more than three orders
- **THEN** thumbnails in cards below the first screen carry `loading="lazy"`

#### Scenario: The detail does not jump when the timeline resolves

- **WHEN** the detail renders before shipment and saga data arrive
- **THEN** a `Skeleton` of the timeline's footprint is shown and replaced without moving the sections below it

### Requirement: Order screens follow the type scale, colour and responsive rules

Order screens SHALL use only the 12/14/16/20/24 type scale (no `text-[Npx]` or other arbitrary values) and
design tokens. Brand colour SHALL appear only on primary CTAs, `PriceTag` and badges/counts; statuses SHALL use
semantic tones. At a 375px viewport the page SHALL have no horizontal page scroll, status tabs SHALL scroll
horizontally inside their own container, order rows and the items `Table` SHALL stack, the `Stepper` SHALL be
vertical, header actions SHALL wrap into full-width buttons with the primary action first, and the Modals SHALL
be full width. At 1024px and wider the content SHALL be at most 960px wide, the `Stepper` horizontal, the items
shown as table columns and header actions inline at the right of the header.

#### Scenario: Mobile layout at 375px

- **WHEN** `/account/orders` and a detail page are opened at a 375px wide viewport
- **THEN** `document.documentElement.scrollWidth` does not exceed the viewport width, the tab bar scrolls inside
  its own container, and the action buttons are full width

#### Scenario: Desktop layout at 1280px

- **WHEN** a detail page is opened at 1280px wide
- **THEN** the `Stepper` is horizontal, the items are shown as table columns and the content column is at most
  960px wide

#### Scenario: No arbitrary type sizes

- **WHEN** the token lint runs over `src/features/order/` and `src/app/account/orders/`
- **THEN** it reports no `text-[..px]`, hex colours or other arbitrary values

### Requirement: Server and client boundaries and Server Action contract

Pages, `OrderList`, `OrderTimeline`, the descriptions and items table SHALL be server components. Client code
SHALL be limited to leaf islands: `OrderStatusTabs`, the detail `Tabs`, `OrderActions` (reorder, cancel and
return triggers with their `Modal`s) and `ReviewModal`. `cancelOrderAction`, `reorderAction`,
`createReturnRequestAction` and `mockRefundAction` SHALL return `{ ok, error?, data? }` and call `revalidatePath`
for `/account/orders` and the affected `/account/orders/[id]`.

#### Scenario: Actions return the standard shape and revalidate

- **WHEN** each of the four actions runs against a mocked gateway for success and for failure
- **THEN** success returns `{ ok: true }` (with `data` where applicable) and calls `revalidatePath`; failure
  returns `{ ok: false, error }` and does not throw
- **VERIFIED BY**: Vitest `team-frontend/src/features/order/actions.test.ts` › cancelOrderAction › cancels with a reason and revalidates the list and the detail; cancelOrderAction › returns the error shape without throwing or revalidating; reorderAction › returns the item count as data and revalidates; reorderAction › returns the error shape when the gateway throws (and team-frontend/src/features/order/returns.actions.test.ts › createReturnRequestAction › creates the return request and returns its view; createReturnRequestAction › returns an error shape when the gateway throws). Not verifiable end to end: the scenario itself runs the actions against a mocked gateway, so it is a unit scenario by construction; `mockRefundAction` named by the requirement no longer exists in `features/order/actions.ts` (spec drift, not covered by any test).

#### Scenario: Pages are not client components

- **WHEN** the source of `app/account/orders/page.tsx` and `app/account/orders/[id]/page.tsx` is inspected
- **THEN** neither contains `"use client"`, and the order data is rendered in the server HTML response

### Requirement: Tracking hooks and test hooks are unchanged

The order screens SHALL NOT add, remove or alter tracking hooks (`TrackLink`, `TrackImpression`,
`SearchImpressions`, `AnalyticsProvider`, recommendation placement attribution) or any `data-*` attribute.
All existing `data-testid` values on these components SHALL be preserved. The impression, click and attribution
events that fire on other routes SHALL still fire after the buyer reorders and lands on `/cart`.

#### Scenario: Test ids and data attributes are preserved

- **WHEN** the existing component tests for `OrderTimeline` and `ReturnRequestSection` are run against the
  rebuilt components
- **THEN** they pass without changes to their selectors (`order-timeline`, `timeline-checkpoint`,
  `timeline-saga`, `timeline-saga-step`, `timeline-empty`, `return-section`, `return-reason`,
  `return-amount`, `return-submit`, `return-status`)

#### Scenario: Tracking events still fire after reorder

- **WHEN** a buyer clicks "Mua lại" and lands on `/cart`
- **THEN** the page view event from `AnalyticsProvider` fires and the cart page's existing tracking events are
  emitted exactly as before this change
