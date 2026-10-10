## Purpose

Defines the seller cockpit of `team-frontend` (`/seller/**`, `/sell`): shell, page anatomy per Ant Design Pro
template, URL state, mutation feedback, loading/empty/error states and responsive behaviour, composed only
from the `ui-components` set and the agora tokens.

## ADDED Requirements

### Requirement: SellerLayout provides a collapsible sidebar and a mobile Drawer

`/seller/**` SHALL render inside `SellerLayout`. At >= 1024px it SHALL show a sidebar (256px expanded, 64px
collapsed) with a collapse toggle, the active route highlighted with `aria-current="page"`, and a shop card
(`Avatar`, the real shop display name, `Tag`; "Shop #" followed by the first 6 characters of the seller id only
when the name is empty). Below 768px it SHALL show a top bar with a menu button that opens the same navigation
in a `Drawer`; the Drawer SHALL close on navigation and on Escape and return focus to the menu button. A
principal without `listing.write` SHALL see a `Result` (403) with a recovery link to `/`, not the cockpit; an
unauthenticated visitor SHALL be redirected to `/login`.

#### Scenario: Sidebar collapses on desktop

- **WHEN** a seller at a 1280px viewport clicks the collapse toggle
- **THEN** the sidebar width becomes 64px, labels are hidden but each link keeps an accessible name, and the
  content area expands

#### Scenario: Navigation opens as a Drawer at 375px

- **WHEN** a seller at a 375px viewport taps the menu button
- **THEN** a Drawer (`role="dialog"`) lists the seller links, the sidebar is not rendered inline, and
  choosing "Quản lý đơn hàng" navigates to `/seller/orders` and closes the Drawer

#### Scenario: Shop card shows the real shop name

- **WHEN** the seller's shop display name is "Cửa hàng Hoa Mai"
- **THEN** the sidebar shop card shows "Cửa hàng Hoa Mai" and not "Shop #"

#### Scenario: Empty shop name falls back

- **WHEN** the shop display name is empty
- **THEN** the shop card shows "Shop #" followed by the first 6 characters of the seller id

#### Scenario: Active route is announced

- **WHEN** the seller is on `/seller/wallet`
- **THEN** only the "Ví người bán" link has `aria-current="page"`

#### Scenario: Missing scope shows a recovery Result

- **WHEN** a logged-in buyer without `listing.write` opens `/seller`
- **THEN** a `Result` explains the seller role is required and offers a link back to `/`

### Requirement: The Workplace dashboard shows KPIs, quick actions and recent orders

`/seller` SHALL render (Dashboard › Workplace): a `SellerPageHeader` with `Breadcrumb`, title and a single
primary "Thêm sản phẩm" `Button`; a `Statistic` row (total products, published, low stock <= 5, open orders);
a quick-actions `Card` linking to new product, orders, wallet, ads and bundles; a recent-orders `Table` (up to
5, linking to `/seller/orders/[id]`); and the product table. The page SHALL be a server component; no
hard-coded metric values are allowed, and a KPI whose data source fails or does not exist SHALL be hidden
rather than shown as a placeholder value.

#### Scenario: KPI row reflects the seller's data

- **WHEN** a seller with 3 published listings, one of them with stock 2, opens `/seller`
- **THEN** the KPI row shows total 3, published 3 and low stock 1, derived from the fetched data

#### Scenario: A KPI without a source is hidden, not zeroed

- **WHEN** the call that feeds a KPI (for example open orders) fails
- **THEN** that `Statistic` cell is not rendered, no placeholder `0` appears in its place, and the other KPIs
  still render

#### Scenario: No hard-coded metric values

- **WHEN** the Workplace is rendered with an empty gateway response
- **THEN** every `Statistic` shows a value derived from that response (total products 0 only because zero
  listings were returned) and no fixed number from source code

#### Scenario: Quick actions navigate

- **WHEN** the seller activates "Ví người bán" in quick actions
- **THEN** the browser navigates to `/seller/wallet`

#### Scenario: Seller with no orders sees an Empty in recent orders

- **WHEN** the seller has no orders
- **THEN** the recent-orders block shows an `Empty` with a link to `/seller/new`, not a blank area

### Requirement: Product and order tables follow Table List with filters in the URL

The product table on `/seller` and the order table on `/seller/orders` SHALL read their state from
`searchParams` (`/seller`: `q`, `status`, `page`; `/seller/orders`: `status`, `q`, `page`), render a `Table`
with row keys and a `Pagination` (links) whose page size is 20, and keep filters, tab and page across reload
and browser back/forward. Invalid parameter values SHALL fall back to defaults. Filter changes SHALL reset
`page` to 1. The filter bar is the only client island on the list (debounced search input); the table and
pagination are server-rendered.

#### Scenario: Search is reflected in the URL

- **WHEN** the seller types "iphone" in the product search box on `/seller`
- **THEN** the URL becomes `/seller?q=iphone` (page reset), and the table shows only matching rows

#### Scenario: Order status tab is a link and survives reload

- **WHEN** the seller selects the "Đang giao" tab on `/seller/orders` and reloads the page
- **THEN** the URL still contains `status=` for that tab, the same tab is selected, and only shipped orders
  are listed

#### Scenario: Pagination moves between pages

- **WHEN** a seller with 45 listings opens `/seller?page=2`
- **THEN** rows 21-40 are shown, `Pagination` marks page 2 current (`aria-current="page"`), and Previous and
  Next links point at `page=1` and `page=3`

#### Scenario: Invalid page falls back

- **WHEN** the seller opens `/seller?page=abc` or `/seller?status=bogus`
- **THEN** the page renders page 1 with the default status, with no error

#### Scenario: No match shows Empty with a clear action

- **WHEN** a filter matches no rows
- **THEN** the table shows an `Empty` ("Không có kết quả") with a "Xoá bộ lọc" link to the unfiltered route

### Requirement: Deleting a listing uses a confirm Modal with pending and toast feedback

The row action "Xoá" SHALL open a `Modal` (`role="dialog"`, `aria-labelledby`) naming the listing and stating
the action cannot be undone, with a cancel `Button` and a danger confirm `Button`. While `deleteListingAction`
is pending the confirm button SHALL be `isLoading` and disabled, and the cancel/close controls disabled. On
`{ ok: true }` the Modal closes, the row disappears (`revalidatePath("/seller")`) and a success toast shows;
on `{ ok: false, error }` the Modal stays open with an `Alert` and an error toast. The native `confirm()` SHALL
NOT be used.

#### Scenario: Delete requires confirmation

- **WHEN** the seller activates "Xoá" on a row
- **THEN** a Modal opens focused on the cancel button and the listing is not deleted until the seller confirms

#### Scenario: Confirm shows pending then success

- **WHEN** the seller confirms deletion
- **THEN** the confirm button shows a spinner and is disabled, and after success the Modal closes, a success
  toast appears and the row is gone from the table

#### Scenario: Failed delete keeps the Modal open

- **WHEN** `deleteListingAction` returns `{ ok: false, error }`
- **THEN** the Modal stays open, shows the error in an `Alert`, an error toast appears and the confirm button
  is enabled again

### Requirement: The listing studio is a validated, pending-aware form

`/seller/new` and `/seller/[id]/edit` SHALL render `ListingForm` as a Basic/Advanced Form of `Card` sections
(basic info, media, price and stock, variants, status) built from `FormItem` with `Input`, `Select` and `Radio`.
Required fields (title, price > 0, stock >= 0, category) SHALL show an inline `FormItem` error after blur or
submit and server-side `SellState` errors on the matching field. Submit SHALL set the primary `Button`
`isLoading` and disable all sections' controls until the action settles, and SHALL show a success toast (and a
`Result` link to the listing on create) or an error `Alert` plus toast. The hidden `imageKeys` and `variants`
inputs and `saveListingAction` contract are unchanged. `/seller/[id]/edit` for a listing the principal does not
own (and without `admin`) SHALL show a 403 `Result`; an unknown id SHALL show `not-found.tsx`.

#### Scenario: Required field error is announced

- **WHEN** the seller submits the form with an empty title
- **THEN** the title `FormItem` shows an error linked by `aria-describedby`, the Input has
  `aria-invalid="true"`, and no request is sent

#### Scenario: Submit is pending and non-repeatable

- **WHEN** the seller submits a valid form
- **THEN** the submit `Button` shows a spinner with unchanged width, is disabled so a second click does
  nothing, and on success a toast "Đã lưu" appears

#### Scenario: Create succeeds and the listing appears in the seller's list

- **WHEN** the seller creates a listing with valid data
- **THEN** a success state offers "Xem danh sách" and the new listing appears on `/seller`

#### Scenario: Server error is surfaced

- **WHEN** `saveListingAction` returns `{ ok: false, message }`
- **THEN** an `Alert` at the top of the form shows the message, an error toast appears, the entered values are
  preserved and the submit button is enabled

#### Scenario: Editing someone else's listing is refused

- **WHEN** a seller opens `/seller/<id>/edit` of a listing owned by another seller
- **THEN** a 403 `Result` with a link back to `/seller` is shown and the form is not rendered

### Requirement: Magic Listing is a controlled, non-destructive assist

The Magic Listing control SHALL call `magicListingAction` with the current title and category, show a pending
state (`Button isLoading`, `Spin` in the suggestion card) and render the result as a suggestion `Card` (title,
description, price range) with an "Áp dụng" `Button` per field and "Áp dụng tất cả". Applying SHALL set the
controlled form state; the component SHALL NOT read or write inputs through `document.getElementById` or
inject a default title. A non-empty field SHALL NOT be overwritten unless the seller applies it. On failure an
`Alert` with a retry `Button` and an error toast SHALL show, and the form stays editable.

#### Scenario: Suggestions are applied only on request

- **WHEN** the seller has typed a title and a description and runs Magic Listing
- **THEN** a suggestion card appears and the typed values are unchanged until the seller activates "Áp dụng"

#### Scenario: Apply fills the controlled fields

- **WHEN** the seller activates "Áp dụng tất cả"
- **THEN** title, description and price fields show the suggestion values and submit sends them

#### Scenario: Magic Listing failure is recoverable

- **WHEN** `magicListingAction` returns `{ ok: false, error }`
- **THEN** an `Alert` with "Thử lại" and an error toast are shown, and the seller can still submit manually

### Requirement: The seller order detail is an Advanced Profile on real data

`/seller/orders/[id]` SHALL be a server component using real order data (`getOrder`,
`getShipmentTracking`), not hard-coded content. It SHALL show `Breadcrumb`, a header `Descriptions` (order id,
created at, payment method, tracking number), a status `Statistic` with `Tag`, a fulfilment `Stepper`
(Pending, Paid, Shipped, Completed; `aria-current="step"` on the current step), `Tabs` for items (`Table`,
line totals with `PriceTag`) and shipment (`Timeline` of checkpoints), and `Descriptions` for recipient,
address and phone. "In phiếu" SHALL print only the packing-slip region. "Bàn giao vận chuyển" SHALL open a
confirm `Modal` and call `updateOrderStatusAction` (pending, disabled, toast, `revalidatePath`), and SHALL be
shown only while the order is shippable. An unknown order or one not belonging to the seller SHALL show
`not-found.tsx`.

#### Scenario: Detail shows the real order

- **WHEN** a seller opens `/seller/orders/<id>` for one of their orders
- **THEN** the recipient, items and totals shown match that order and no hard-coded sample buyer or product
  appears

#### Scenario: Shipping an order gives feedback

- **WHEN** the seller confirms "Bàn giao vận chuyển" on a paid order
- **THEN** the confirm button is pending then disabled, a success toast appears, and the status `Tag` and
  `Stepper` show Shipped after revalidation

#### Scenario: Shipping failure is reported

- **WHEN** `updateOrderStatusAction` returns `{ ok: false, error }`
- **THEN** an error toast and an `Alert` with the error are shown and the order status is unchanged

#### Scenario: Unknown order is a not-found state

- **WHEN** a seller opens `/seller/orders/does-not-exist`
- **THEN** the seller `not-found.tsx` renders a `Result` with a link to `/seller/orders`

#### Scenario: Print hides chrome

- **WHEN** the seller activates "In phiếu"
- **THEN** the print stylesheet hides the sidebar, header actions and toasts and prints the packing slip only

### Requirement: Analysis page shows KPIs, funnel and revenue with a URL range

`/seller/analytics` SHALL render (Dashboard › Analysis) a `Statistic` row (revenue, orders, impressions,
conversion), a funnel `Card` with `Progress` rows for impressions, views, adds and orders from
`getSellerFunnel`, and a revenue-by-day `Table` from `getRevenueBreakdown`, with range `Tabs` (`7d`, `30d`,
`90d`) in `?range=`. It SHALL NOT render mock data (`SellerAnalyticsMock` is removed). A gateway error SHALL
show an `Alert` with a retry link, and an empty series an `Empty`.

#### Scenario: Range tab changes the data via the URL

- **WHEN** the seller selects "30 ngày"
- **THEN** the URL contains `range=30d` and the revenue table and totals cover the last 30 days

#### Scenario: Metrics summary is visible

- **WHEN** the seller opens `/seller/analytics`
- **THEN** the dashboard and the revenue metrics summary (`Statistic` row) are visible

#### Scenario: Analytics failure is recoverable

- **WHEN** `getSellerFunnel` throws
- **THEN** an `Alert` with a "Thử lại" link to the same URL is shown and the rest of the page still renders

### Requirement: Wallet, plans, ads and bundles use Server Actions with the shared mutation contract

`/seller/wallet`, `/seller/plans`, `/seller/ads` and `/seller/bundles` SHALL be server pages composed of
`Statistic`, `Card`, `Table`, `Pagination` and `FormItem`-based forms. Payout, subscribe, create-ad-campaign
and create-bundle SHALL be Server Actions returning `{ ok: boolean; error?: string; data?: T }` that call
`revalidatePath` for their route; their client buttons SHALL show a pending state, be disabled while pending
and when the action is not applicable (zero balance, current plan, fewer than 2 bundle items), and show a
success or error toast. Payout SHALL require a confirm `Modal`. Plans SHALL mark the current plan with a `Tag`.

#### Scenario: Payout confirms, pends and toasts

- **WHEN** a seller with a positive balance activates "Rút tiền" and confirms
- **THEN** the confirm button is pending then disabled, a success toast appears and the balance and ledger
  refresh

#### Scenario: Payout is disabled without balance

- **WHEN** the balance is 0
- **THEN** the payout `Button` is disabled with `aria-disabled="true"` and explains why

#### Scenario: Current plan cannot be re-subscribed

- **WHEN** the seller views `/seller/plans`
- **THEN** the current plan shows a "Gói hiện tại" `Tag` and a disabled button, other plans show an enabled
  "Đăng ký" button

#### Scenario: Action errors use the error field

- **WHEN** `createBundleAction` returns `{ ok: false, error: "Chọn ít nhất 2 sản phẩm cho combo." }`
- **THEN** the field-level `FormItem` or an `Alert` shows that message and an error toast appears

#### Scenario: Bundle creation succeeds

- **WHEN** the seller creates a valid bundle
- **THEN** a success toast appears, the form resets and the bundle appears in the list after revalidation

### Requirement: The seller can set the shop display name

The seller area SHALL provide a shop profile form (a `/seller/shop` page linked from the sidebar, Ant Pro
Basic Form) with a `FormItem` for the shop display name, saved through a Server Action that calls the
gateway `UpsertStorefront` with `display_name` (contract from `shop-display-name`) and returns
`ActionResult`. The field SHALL be trimmed, required to be 1–80 characters, show inline validation, a
pending submit, and a success or error toast, and SHALL be pre-filled with the current name.

#### Scenario: Saving a new shop name is reflected on the storefront

- **WHEN** a seller enters "Nhà Sách An Nhiên" and submits the shop profile form
- **THEN** the submit button shows pending, a success toast appears, and `/shop/<sellerId>` and the PDP
  shop header show "Nhà Sách An Nhiên"

#### Scenario: An empty or too long name is rejected inline

- **WHEN** a seller submits a blank name or one longer than 80 characters
- **THEN** no request is sent and the field shows an inline error linked by `aria-describedby`

### Requirement: Every seller route defines loading, empty, error and not-found states

Each seller route segment SHALL provide a `loading.tsx` whose `Skeleton` has the same footprint as the final
layout (KPI row, filter bar, table rows, form sections), an `error.tsx` rendering a `Result` with a retry
`Button` (`reset`) and a link to `/seller`, and a `not-found.tsx` (segment level) rendering a `Result` with a
link to `/seller`. Tables SHALL show `Empty` with a primary recovery action when there is no data and an
`Alert` with retry when a gateway read fails.

#### Scenario: Skeleton matches the page footprint

- **WHEN** `/seller` is requested while its data is pending
- **THEN** a skeleton with the KPI row and table rows is shown, and when data arrives the page content
  height does not shift (CLS = 0)

#### Scenario: Route error offers recovery

- **WHEN** a seller page throws during render
- **THEN** `error.tsx` shows a `Result` with "Thử lại" calling `reset` and a link to `/seller`

#### Scenario: Empty product list shows a call to action

- **WHEN** a seller has no listings
- **THEN** the product table area shows `Empty` ("Shop chưa có sản phẩm") with a primary "Thêm sản phẩm" link

#### Scenario: List read failure shows an Alert

- **WHEN** `listMyListings` throws
- **THEN** an `Alert` with a retry link is shown above an empty table instead of a raw error string

### Requirement: Seller pages follow the zero-layout-shift rules

Product and listing images SHALL use `Image` with a fixed aspect (`aspect-square`, 1:1) and a fallback;
images below the first screen (table thumbnails past the first rows, media grid) SHALL be lazy loaded; async
blocks SHALL render a `Skeleton` fallback via `loading.tsx` or `Suspense`; `Statistic` with `loading` SHALL
keep its height; the shell SHALL NOT change content width after hydration on mobile.

#### Scenario: Thumbnails reserve their space

- **WHEN** the product table renders listings with slow-loading images
- **THEN** each thumbnail box is already 1:1 and the row height does not change when the image loads

#### Scenario: Below-the-fold images are lazy

- **WHEN** a seller has 20 listings on the page
- **THEN** thumbnails after the first screen have `loading="lazy"`

### Requirement: Type scale and brand colour follow the design system

Seller pages SHALL use only the 12/14/16/20/24px type scale (no text below 12px) and Tier 2 colour aliases
(no raw hex or arbitrary `-[..]` values). Brand colour SHALL appear only on the single primary CTA of a page,
on prices (`PriceTag`) and on badges; success and danger states SHALL use the `success` / `danger` aliases.

#### Scenario: Token lint passes on seller code

- **WHEN** `npm run check` runs after the change
- **THEN** the token lint reports no raw hex, `rgb()` or arbitrary `-[..]` value in `src/app/seller/**`,
  `src/app/sell/**` or `src/features/seller/**`

#### Scenario: One primary CTA per page

- **WHEN** `/seller/orders` is rendered
- **THEN** at most one brand-filled primary `Button` is visible in the page header and other actions are
  `outline` or `ghost`

### Requirement: Layouts are responsive at 375px and desktop

At 375px the seller pages SHALL have no horizontal page scroll: KPI `Statistic` cells in a two-column grid,
tables scrolling inside their own `overflow-x-auto` container, the filter bar stacked, form actions in a sticky
bottom bar, and `Pagination` showing previous, current and next only. At >= 1024px the layout SHALL show the
sidebar, a four-up KPI row and full tables.

#### Scenario: No page-level horizontal scroll at 375px

- **WHEN** `/seller`, `/seller/orders` and `/seller/orders/<id>` are opened at a 375px viewport
- **THEN** `document.documentElement.scrollWidth` equals the viewport width

#### Scenario: Form actions stay reachable on mobile

- **WHEN** the listing studio is scrolled to the middle at 375px
- **THEN** the Save button remains visible in the sticky bottom bar

#### Scenario: Desktop shows a four-up KPI row

- **WHEN** `/seller` is opened at 1280px
- **THEN** the four KPI `Statistic` cells appear in one row next to the sidebar

### Requirement: Tracking hooks and public links stay unchanged

The seller routes SHALL NOT add, remove or alter `TrackLink`, `TrackImpression`, `SearchImpressions`,
`AnalyticsProvider`, recommendation placement attribution or any `data-*` attribute used by tracking. Product
links to `/listing/[id]` and the public shop link SHALL remain plain links to the same URLs, and the analytics
data read by the Analysis page SHALL still come from `getSellerFunnel` and `getRevenueBreakdown`.

#### Scenario: Impression, click and attribution events still fire

- **WHEN** a buyer session views a listing, clicks it from search and adds it to cart after this change
- **THEN** the tracking events (impression, click with placement attribution) are still emitted with the same
  payload shape, and the seller funnel on `/seller/analytics` counts them

#### Scenario: Row links keep their targets

- **WHEN** the seller activates the title link of a product row
- **THEN** the browser opens `/listing/<id>` exactly as before

### Requirement: `/sell` keeps redirecting to the listing studio

`/sell` SHALL remain a redirect to `/seller/new` (server `redirect`, no client code).

#### Scenario: /sell redirects

- **WHEN** a seller opens `/sell`
- **THEN** the browser lands on `/seller/new`
