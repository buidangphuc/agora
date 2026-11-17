# ui-seller Specification

## Purpose
Defines the seller cockpit of `team-frontend` (`/seller/**`, `/sell`): shell, page anatomy per Ant Design Pro
template, URL state, mutation feedback, loading/empty/error states and responsive behaviour, composed only
from the `ui-components` set and the agora tokens.

## Requirements

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
`Statistic`, `Card`, `Table`, `Pagination` and `FormItem`-based forms. Payout, subscribe, cancel-subscription,
create-ad-campaign and create-bundle SHALL be Server Actions returning `{ ok: boolean; error?: string; data?: T }`
that call `revalidatePath` for their route; their client buttons SHALL show a pending state, be disabled while
pending and when the action is not applicable (zero balance, current plan, fewer than 2 bundle items), and show a
success or error toast. Payout SHALL require a confirm `Modal`; subscribe and cancel-subscription SHALL require a
confirm `Modal` that states the charge or the period end. Plans SHALL mark the current plan with a `Tag`.

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
  "Nâng cấp" button above the current tier and an enabled "Hạ gói" button below it

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
- **THEN** the token lint reports no raw hex, `rgb()` or arbitrary `-[..]` value in `team-frontend-seller`
  `src/app/seller/**`, `src/app/sell/**` or `src/features/seller/**`

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

### Requirement: The seller ads page lists campaigns from the service

`/seller/ads` SHALL list the signed-in seller's ad campaigns from `SponsoredService.ListMyAdCampaigns`, 20 per page
with the page in the URL, each row showing the listing title, bid, budget, spent, remaining budget, impressions,
clicks and a status label, all money as integer VND. The list SHALL survive a reload and SHALL include a campaign
immediately after it is created. A failed list call SHALL affect only the list.

#### Scenario: Campaigns persist after reload

- **WHEN** a seller who created a campaign with budget 50 000 and bid 1 000 reloads `/seller/ads`
- **THEN** the table shows one row with "₫1.000" bid, "₫50.000" budget, "₫0" spent, "₫50.000" remaining, 0 impressions, 0 clicks and "Đang chạy"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Campaigns persist after reload`

#### Scenario: Spend and counters come from the service

- **WHEN** a buyer's click has charged the campaign once at bid 1 000 and the seller opens `/seller/ads`
- **THEN** the row shows "₫1.000" spent, "₫49.000" remaining, 1 click and at least 1 impression
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Spend and counters come from the service`

#### Scenario: Seller with no campaign sees an empty state

- **WHEN** a seller with no campaigns opens `/seller/ads`
- **THEN** the table shows "Chưa có chiến dịch nào" and the create form is still rendered
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `renders empty state`

#### Scenario: Campaigns are paged at 20

- **WHEN** a seller has 21 campaigns and opens `/seller/ads` and then `/seller/ads?page=2`
- **THEN** the first page shows 20 rows and the second shows 1 row
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/ads/page.test.tsx` `pages campaigns at 20`

#### Scenario: List failure degrades only the list

- **WHEN** `ListMyAdCampaigns` fails with UNAVAILABLE or exceeds the 2 s timeout
- **THEN** the list shows an error alert "Không tải được chiến dịch" with a "Thử lại" link, the create form renders, and the page responds 200
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/ads/page.test.tsx` `list error degrades only the list`

#### Scenario: Guest and non-seller are not authorized to open the page

- **WHEN** an unauthenticated guest (401) opens `/seller/ads`, and separately a signed-in buyer without `listing.write` opens it
- **THEN** the guest is redirected to the login page with `/seller/ads` as the return path, the buyer sees no campaign data, and no `ListMyAdCampaigns` call is made for either
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Guest and non-seller are not authorized to open the page`

### Requirement: A seller can pause and resume a campaign

Each campaign row SHALL offer "Tạm dừng" when the status is ACTIVE and "Tiếp tục" when it is PAUSED, calling
`PauseAdCampaign` or `ResumeAdCampaign` through a Server Action that returns `{ ok, error?, data? }`, shows a
pending state, is disabled while pending, toasts the outcome and refreshes the row from the service response. A
campaign in `PAUSED_BUDGET` SHALL show "Hết ngân sách" with the resume button disabled, and an ENDED campaign
SHALL show no action. A campaign of another seller SHALL NOT be actionable: the service returns NOT_FOUND and the
UI reports "Không tìm thấy chiến dịch.".

#### Scenario: Pause an active campaign

- **WHEN** the seller activates "Tạm dừng" on an ACTIVE campaign
- **THEN** the row status becomes "Tạm dừng" and the button becomes "Tiếp tục", still so after reload, and the campaign is no longer served to buyers
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Pause an active campaign`

#### Scenario: Resume a paused campaign

- **WHEN** the seller activates "Tiếp tục" on a PAUSED campaign whose remaining budget is at least its bid
- **THEN** the row status becomes "Đang chạy" and the button becomes "Tạm dừng", still so after reload
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Resume a paused campaign`

#### Scenario: Budget-exhausted campaign cannot resume

- **WHEN** a campaign has spent so that remaining budget (₫400) is below its bid (₫500 or more) and the seller opens `/seller/ads`
- **THEN** the row shows "Hết ngân sách" and the resume button is disabled, and if the action is still forced the error toast reads "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy." and the status is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `resume maps FAILED_PRECONDITION to the budget message`

#### Scenario: Ended campaign has no action

- **WHEN** a campaign has status ENDED
- **THEN** its row shows "Đã kết thúc" and neither "Tạm dừng" nor "Tiếp tục" is rendered
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `ended row has no action`

#### Scenario: Double click issues one call

- **WHEN** the seller double-clicks "Tạm dừng"
- **THEN** the button is disabled after the first click and exactly one `PauseAdCampaign` call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `double click pauses once`

#### Scenario: Another seller's campaign is not found

- **WHEN** seller B submits Pause or Resume with the id of seller A's campaign
- **THEN** the action returns `ok: false` with "Không tìm thấy chiến dịch.", A's campaign is unchanged, and B's list does not contain it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Another seller's campaign is not found`

### Requirement: The campaign form enforces the minimum bid

The create form and `createAdCampaignAction` SHALL reject a per-click bid below 500 VND, and a budget below the
bid, with a field error and without calling the service from the client path; a bid of exactly 500 VND SHALL be
accepted. Server INVALID_ARGUMENT SHALL still be shown as the form error.

#### Scenario: Bid below minimum is rejected

- **WHEN** the seller submits a campaign with bid 499
- **THEN** the field error "Giá thầu tối thiểu là ₫500." is shown and no `CreateAdCampaign` call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerForms.test.tsx` `rejects bid below 500`

#### Scenario: Bid of exactly 500 is accepted

- **WHEN** the seller submits a campaign with bid 500 and budget 5 000
- **THEN** the campaign is created and appears in the list as "Đang chạy" with "₫500" bid
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Bid of exactly 500 is accepted`

#### Scenario: Ad form: budget below the bid is rejected

- **WHEN** the seller submits bid 1 000 and budget 800
- **THEN** the field error "Ngân sách phải lớn hơn hoặc bằng giá thầu." is shown and no call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerForms.test.tsx` `rejects budget below bid`

### Requirement: The seller order page shows the settlement breakdown

`/seller/orders/[id]` SHALL show, for an order that has settled into the seller's wallet, a "Đối soát thanh toán" card
with the amount credited, the commission and its rate, the commission returned by refunds, the refunds deducted and
the net, each formatted as integer VND, using only the values returned by `PaymentService.GetOrderSettlement`. The
page SHALL request the settlement only after the existing ownership check passes. For an order that has not settled
the card SHALL say so, and a failed settlement call SHALL affect only the card.

#### Scenario: Breakdown of a settled order

- **WHEN** the owning seller opens `/seller/orders/[id]` of an order paid online with items_subtotal 300 000,
  shipping 30 000 and shop voucher 20 000 (credited 310 000, commission 14 000 at 500 bps)
- **THEN** the card shows "₫310.000" as credited, "₫14.000" as commission labelled "5%", "₫296.000" as net, and
  commission returned and refunds deducted as "₫0"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Breakdown of a settled order`

#### Scenario: Breakdown after a partial refund

- **WHEN** the owning seller opens the order page after a 40% partial return refund has been paid
  (commission_reversed 5 600, refund_deducted 124 000)
- **THEN** the card shows commission returned "₫5.600", refunds deducted "₫124.000" and net "₫177.600", equal to
  the response `net`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Breakdown after a partial refund`

#### Scenario: Zero commission is shown, not hidden

- **WHEN** the response has commission 0 and commission_rate_bps 0 (an order settled before commission existed)
- **THEN** the card shows commission "₫0" with rate "0%" and net equal to credited
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/OrderSettlementCard.test.tsx` `Zero commission is shown, not hidden: renders 0% and a zero amount`

#### Scenario: Unsettled order shows the not-settled state

- **WHEN** the owning seller opens the page of a cash-on-delivery order, whose `GetOrderSettlement` returns NOT_FOUND
- **THEN** the card shows "Đơn chưa được đối soát vào ví" and no amount rows, and the rest of the order page renders
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Unsettled order shows the not-settled state`

#### Scenario: Settlement service failure degrades only the card

- **WHEN** `GetOrderSettlement` fails with UNAVAILABLE or exceeds the 2 s timeout while the order itself loads
- **THEN** the card shows an error alert "Không tải được đối soát thanh toán" with a "Thử lại" link, the order items,
  totals and tabs still render, and the page responds 200
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/[id]/page.test.tsx` `settlement error degrades only the card`

#### Scenario: Another seller's order is not found and nothing is fetched

- **WHEN** a seller opens `/seller/orders/[id]` of an order that belongs to a different seller
- **THEN** the response is the not-found page and no `GetOrderSettlement` call is made
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Another seller's order is not found and nothing is fetched`

#### Scenario: Guest cannot see the breakdown

- **WHEN** a visitor without a session requests `/seller/orders/[id]`
- **THEN** the visitor is redirected to login and the response contains no settlement amounts
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Guest cannot see the breakdown`

#### Scenario: Large amounts keep integer precision

- **WHEN** the response holds credited 9 007 199 254 740 000 (below 2^53) and commission 450 359 962 737 000
- **THEN** the card renders both exactly as formatted integer VND with no fractional digit
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/payment.test.ts` `maps int64 settlement amounts without rounding`

### Requirement: The wallet ledger shows Vietnamese type labels

The `/seller/wallet` ledger Table SHALL show a Vietnamese label for each wallet transaction type: ORDER_SETTLEMENT
"Thanh toán đơn hàng", PAYOUT "Rút tiền", REFUND_DEDUCTION "Hoàn tiền cho người mua", COMMISSION "Phí hoa hồng",
COMMISSION_REVERSAL "Hoàn phí hoa hồng", SUBSCRIPTION_FEE "Phí gói người bán", AD_SPEND "Chi phí quảng cáo". A type
the UI does not know SHALL be shown as its raw string, and an empty type as "—".

#### Scenario: Commission rows are labelled

- **WHEN** the seller opens `/seller/wallet` after an order settled with commission 14 000
- **THEN** the ledger shows a row "Thanh toán đơn hàng" and a row "Phí hoa hồng" with amount "-₫14.000", and the text
  "COMMISSION" appears in no cell
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Commission rows are labelled`

#### Scenario: Every known type has a label

- **WHEN** the label function receives each of the seven known type strings
- **THEN** it returns the Vietnamese label listed above for each, and none returns the raw string
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ledgerLabels.test.ts` `labels every known type`

#### Scenario: Unknown or empty type falls back

- **WHEN** a ledger entry has type "FUTURE_TYPE" or an empty type
- **THEN** the cell shows "FUTURE_TYPE" or "—" respectively and the page does not error
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ledgerLabels.test.ts` `falls back for unknown and empty types`

#### Scenario: Ledger load failure keeps the existing error state

- **WHEN** `ListLedgerEntries` fails while the wallet page loads
- **THEN** the page shows "Không tải được lịch sử giao dịch" with "Thử lại" and no label is rendered from stale data
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/wallet/wallet.test.tsx` `ledger error keeps the retry state`

#### Scenario: Guest cannot see the ledger

- **WHEN** a visitor without a session requests `/seller/wallet`
- **THEN** the visitor is redirected to login and the response contains no ledger rows
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/wallet/wallet.test.tsx` `guest is redirected to login`
