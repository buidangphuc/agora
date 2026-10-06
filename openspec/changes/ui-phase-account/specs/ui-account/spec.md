## Purpose

Defines the account and authentication screens of `team-frontend` (`/login`, `/register`,
`/account/addresses`, `/account/security`, `/account/verification`, `/account/referral`,
`/account/following`, `/favorites`, `/notifications`), modelled on Ant Design Pro's Account Settings,
Account Center and Login/Register templates and built only from the core components of `ui-components`.

## ADDED Requirements

### Requirement: Account pages share a URL-driven settings shell

The five `/account/addresses`, `/account/security`, `/account/verification`, `/account/referral` and
`/account/following` pages SHALL render inside a server `AccountShell` with a `Breadcrumb`, an `h1` (24px),
and a navigation menu whose items are links. The active item SHALL be derived from the route (not client
state) and carry `aria-current="page"`. On desktop the menu is a 240px left column; at 375px it is a single
horizontally scrollable row above the content with no page-level horizontal scroll.

#### Scenario: The menu reflects the URL

- **WHEN** a signed-in buyer opens `/account/security`
- **THEN** the "Bảo mật" menu item has `aria-current="page"`, the others do not, and the `h1` is "Bảo mật
  tài khoản"

#### Scenario: Menu navigation is a real navigation

- **WHEN** the buyer clicks "Xác minh" in the menu and then uses the browser Back button
- **THEN** the URL becomes `/account/verification` and Back returns to `/account/security` with that page
  re-rendered

#### Scenario: Menu collapses on mobile

- **WHEN** the viewport is 375px wide
- **THEN** the menu renders as one scrollable row above the content and `document.documentElement.scrollWidth`
  does not exceed the viewport width

### Requirement: Account pages require a session

Every account page except `/login` and `/register` SHALL redirect an anonymous visitor to `/login` without
rendering account data.

#### Scenario: Anonymous visitor is redirected

- **WHEN** an anonymous visitor opens `/account/addresses`
- **THEN** they are redirected to `/login` and no address data is in the response

### Requirement: Login and register use validated FormItems with an inline error Alert

`/login` and `/register` SHALL render a centred `Card` containing one `FormItem` per field (label bound to
the control, `required` mark, `help` text), preserving the control `name`s `username`, `password` and (register)
`role`. The submit `Button` SHALL show `isLoading` and be disabled while the action is pending. Failures SHALL
render an `Alert type="error"` with `role="alert"` above the submit button; success redirects as today.

#### Scenario: Required fields are validated before submit

- **WHEN** a visitor submits `/login` with an empty username
- **THEN** no request is sent, the username `FormItem` shows help text and `aria-invalid="true"`, and focus
  moves to the username input

#### Scenario: Pending submit is visible and inert

- **WHEN** a visitor submits valid credentials
- **THEN** the submit button shows a spinner, has `aria-busy="true"`, keeps its width, and a second click does
  not send a second request

#### Scenario: Wrong credentials show an inline error

- **WHEN** the gateway rejects the credentials
- **THEN** an error `Alert` with `role="alert"` reading "Tên đăng nhập hoặc mật khẩu không chính xác." is
  shown, the typed username is kept, the password is cleared, and the user remains on `/login`

#### Scenario: Register enforces the minimum lengths

- **WHEN** a visitor enters a 2-character username or a 3-character password on `/register`
- **THEN** the matching `FormItem` shows its minimum-length help text and the form is not submitted

### Requirement: Address book supports add, edit, delete and set-default with feedback

`/account/addresses` SHALL list addresses as cards (recipient, phone, full address, a "Mặc định" `Tag` on the
default) with a primary "Thêm địa chỉ mới" `Button`. Create and edit SHALL open a `Modal` form built from
`FormItem`/`Input`; delete SHALL open a confirm `Modal` (no `window.confirm`). Every action SHALL be a Server
Action returning `{ ok, error?, data? }`, call `revalidatePath("/account/addresses")`, show a pending state on
its button, and finish with a success or error toast.

#### Scenario: Adding an address

- **WHEN** a buyer submits a valid address in the add `Modal`
- **THEN** the submit button is pending during the request, the modal closes, a success toast appears and the
  new address card is in the list

#### Scenario: Delete requires confirmation

- **WHEN** a buyer clicks "Xóa" on a non-default address
- **THEN** a confirm `Modal` names the recipient, and the address is deleted only after the confirm button is
  pressed; cancelling or pressing Escape leaves the list unchanged and returns focus to "Xóa"

#### Scenario: A failed mutation reports an error and keeps state

- **WHEN** set-default fails because the gateway returns an error
- **THEN** an error toast is shown, the previous default keeps its "Mặc định" `Tag`, and the button is enabled
  again

#### Scenario: Empty address book

- **WHEN** the buyer has no addresses
- **THEN** an `Empty` block with the description "Bạn chưa có địa chỉ nhận hàng nào." and a "Thêm địa chỉ ngay"
  action is shown

### Requirement: Security page lists sessions and login history in Tables

`/account/security` SHALL show "Phiên đăng nhập" and "Lịch sử đăng nhập" as `Table`s (device/IP/last seen,
user-agent/IP/time/result). Login result SHALL render as a `Tag` (Thành công success, Thất bại danger). Revoke
SHALL open a confirm `Modal`, use a pending state, call a Server Action that revalidates the page, and end
with a toast. Rows beyond 20 SHALL be paginated with `Pagination` via `?page=`.

#### Scenario: Revoking a session

- **WHEN** a buyer confirms "Thu hồi" on an active session
- **THEN** the button is pending, a success toast appears, and after revalidation that row shows "Đã thu hồi"
  with no Revoke button

#### Scenario: Empty history

- **WHEN** the buyer has no login history rows
- **THEN** the history `Table` renders an `Empty` with "Chưa có lịch sử đăng nhập."

#### Scenario: Gateway read failure

- **WHEN** the sessions request fails
- **THEN** the sessions section shows an inline `Alert type="error"` with a "Thử lại" action that reloads the
  route, and the history section still renders

### Requirement: KYC form is fully labelled and status is a Tag

`/account/verification` SHALL show the current status as `Descriptions` with a `Tag` (Verified success,
Pending promo, Rejected danger, Not submitted neutral) whose text states the status, and a verified-badge
line when `badge` is true. `SubmitKycForm` SHALL bind every control to its label through `FormItem`
(`htmlFor`/`id`), expose help as `aria-describedby`, and disable submit until the document reference is
non-empty. Submit SHALL show pending, then success or error toast, and revalidate the page.

#### Scenario: Every KYC control has an accessible name

- **WHEN** the verification page is audited with the accessibility tree
- **THEN** the document-type select is named "Loại giấy tờ" and the reference input is named "Mã tham chiếu
  tài liệu"

#### Scenario: Submitting a KYC document

- **WHEN** a buyer enters a reference and presses "Gửi hồ sơ xác minh"
- **THEN** the button is pending, a success toast appears, the field is cleared and the status `Tag` shows
  "Chờ duyệt" after revalidation

#### Scenario: Status is not conveyed by colour alone

- **WHEN** the status is Verified
- **THEN** the `Tag` contains the text from `statusText` in addition to its colour

### Requirement: Referral page uses Statistic, Descriptions and Timeline

`/account/referral` SHALL show the referral code (brand-coloured, with a copy `Button`), `Statistic`s for
invited count and total rewards (`PriceTag`/price role), a redeem `FormItem` form, and the rewards ledger as a
`Timeline` with an `Empty` state. Generate-code and redeem SHALL be Server Actions with pending state and
toasts and SHALL revalidate `/account/referral`.

#### Scenario: Generating a code

- **WHEN** a buyer without a code presses "Tạo mã"
- **THEN** the button is pending, a success toast appears and the code is displayed after revalidation

#### Scenario: Redeeming an invalid code

- **WHEN** a buyer redeems a code the server rejects
- **THEN** an error toast is shown and the redeem `FormItem` displays the server message as help with
  `aria-invalid="true"`

#### Scenario: No rewards yet

- **WHEN** the rewards list is empty
- **THEN** the `Timeline` area renders `Empty` "Chưa có phần thưởng nào."

### Requirement: Following page is URL-tabbed and skeleton-backed

`/account/following` SHALL render a summary `Card` and `Tabs` for "Gian hàng" and "Sản phẩm" selected by
`?tab=shops|items` (default `shops`). Shop cards use `Avatar`, show the real shop display name (fallback "Shop #" followed by the first 6
characters of the id only when the name is empty), and link to `/shop/{id}`; products link to
`/listing/{id}`. The product feed SHALL load behind Suspense with a `Skeleton` of the final list height.

#### Scenario: Tab is held in the URL

- **WHEN** a buyer opens `/account/following?tab=items`
- **THEN** the "Sản phẩm" tab is selected (`aria-selected="true"`) on first render without client JS

#### Scenario: Shop cards show the real shop name

- **WHEN** a buyer follows a shop whose display name is "Cửa hàng Hoa Mai"
- **THEN** the shop card shows "Cửa hàng Hoa Mai" and not "Shop #"

#### Scenario: Empty shop name falls back

- **WHEN** a followed shop has an empty display name
- **THEN** its card shows "Shop #" followed by the first 6 characters of the shop id

#### Scenario: Empty follow list

- **WHEN** the buyer follows no shops
- **THEN** the shops tab shows `Empty` "Bạn chưa theo dõi gian hàng nào." with a "Khám phá gian hàng" action
  linking to `/search`

### Requirement: Favorites page keeps URL state and reuses ListingGrid unchanged

`/favorites` SHALL show a header with the total as a `Badge`, the collections manager, and the favourites in
the existing `ListingGrid`, filtered by `?collection=<id>` and paginated by `?page=N` with `Pagination`. When
there are no items it SHALL show `Empty` (collection-specific text) with a "Khám phá sản phẩm ngay" action. An
unknown collection id SHALL render a `Result status="404"` with a link back to `/favorites`.

#### Scenario: Collection filter in the URL

- **WHEN** a buyer selects a collection
- **THEN** the URL becomes `/favorites?collection=<id>`, the header shows the collection name, and reloading
  the page shows the same items

#### Scenario: Unknown collection

- **WHEN** a buyer opens `/favorites?collection=does-not-exist`
- **THEN** a `Result` explains the collection was not found and offers "Xem tất cả sản phẩm yêu thích"

#### Scenario: Collection management gives feedback

- **WHEN** a buyer creates a collection
- **THEN** the create button is pending and disabled, a success toast appears, and the new collection is listed

### Requirement: Notifications tabs are URL state and the list is server-rendered

`/notifications` SHALL filter by `?tab=` (all, order, chat, alert, system) and paginate by `?page=` (20 per
page). Tab labels show counts via `Badge`; unread rows are marked by a brand `Badge` dot with a visually
hidden "Chưa đọc" label. The list SHALL be rendered on the server; only the preferences form and the alert
subscriptions are client islands. There SHALL be no "Mark all read" control (no backend); nothing about read
state SHALL be simulated locally. The preferences form SHALL use `Checkbox`,
`Select` and `FormItem`, with pending state and a toast on save.

#### Scenario: Tab survives reload and sharing

- **WHEN** a buyer selects the "Đơn hàng" tab and reloads
- **THEN** the URL contains `?tab=order`, the tab is selected, and only order notifications are listed

#### Scenario: No mark-all-read control

- **WHEN** `/notifications` renders a list with unread items
- **THEN** no "Đánh dấu đã đọc" or equivalent mark-all-read control is rendered

#### Scenario: Empty tab

- **WHEN** a tab has no notifications
- **THEN** `Empty` "Chưa có thông báo nào trong mục này." is shown inside the list area

#### Scenario: Saving preferences

- **WHEN** a buyer changes a toggle and presses "Lưu"
- **THEN** the button is pending, then a success toast appears; on failure an error toast appears and the
  toggles keep the unsaved values

### Requirement: Every account route has a loading and an error state

Each of the nine routes SHALL provide a route `loading.tsx` whose `Skeleton` matches the final footprint
(shell, header and card heights), so content replaces it without layout shift, and SHALL surface route-level
failures through `error.tsx` as `Result status="error"` with a "Thử lại" action. Images (shop avatars) SHALL
use `Image` with a fixed aspect; images below the fold SHALL be lazy.

#### Scenario: Skeleton matches the content

- **WHEN** `/account/security` is loaded with a throttled gateway
- **THEN** a `Skeleton` with the shell and two section placeholders is shown first and the measured
  Cumulative Layout Shift for the page is 0

#### Scenario: Route error offers recovery

- **WHEN** a gateway call throws during rendering of `/notifications`
- **THEN** a `Result status="error"` is shown with a "Thử lại" button that re-renders the route

### Requirement: Mutations return a uniform result and always give feedback

Every Server Action used by these routes SHALL return `{ ok: boolean; error?: string; data?: T }` (with
`loginAction`/`registerAction` allowed to `redirect()` on success) and call `revalidatePath` for the route
that shows the changed data. Every control that triggers one SHALL show a pending state, be disabled while
pending, and end with a success or error toast (or inline `Alert` for the login/register forms).

#### Scenario: Action result shape

- **WHEN** any in-scope Server Action fails with a gateway error
- **THEN** it resolves (does not throw) with `{ ok: false, error }` where `error` is a user-readable string

#### Scenario: Double submit is blocked

- **WHEN** a user double-clicks any in-scope mutation button
- **THEN** exactly one request is sent

### Requirement: Type scale and brand colour discipline

These screens SHALL use only the 12/14/16/20/24px type scale and Tier 2/3 tokens (no raw hex or arbitrary
values), and SHALL use the brand colour only for primary CTAs, prices/reward amounts, the referral code and
unread badges.

#### Scenario: Token lint is clean

- **WHEN** `scripts/check-tokens.mjs` runs on the files of this change
- **THEN** it reports 0 violations

#### Scenario: Status colours are semantic

- **WHEN** the KYC status is Verified, Pending or Rejected
- **THEN** the `Tag` uses the success, promo or danger token respectively and not the brand colour

### Requirement: Responsive layout at mobile and desktop

All nine routes SHALL be usable at 375px and at 1280px: at 375px content is a single column with no
horizontal page scroll, form buttons are full-width, interactive targets are at least 40px high, and `Modal`s
fill the width; at 1280px the settings pages show the 240px menu beside a `max-w-3xl` content column.

#### Scenario: No horizontal scroll on mobile

- **WHEN** each of the nine routes is rendered at 375px wide for a signed-in buyer
- **THEN** `document.documentElement.scrollWidth` is not greater than 375

#### Scenario: Desktop layout

- **WHEN** `/account/addresses` is rendered at 1280px wide
- **THEN** the menu column is visible to the left of the content

### Requirement: Tracking hooks are unchanged

The redesign SHALL NOT alter impression, click or attribution tracking: `ListingGrid`/`ListingCard` on
`/favorites` keep their `TrackLink`/`TrackImpression` behaviour and `data-*` attributes, notification rows
keep `data-testid="notification-item"` and `data-type`, alert rows keep `data-testid="alert-subscription"`
and `data-type`, and `AnalyticsProvider` still wraps the pages.

#### Scenario: Favorites impressions and clicks still fire

- **WHEN** `/favorites` renders a grid of listings and the buyer clicks one
- **THEN** the same impression events and the same click event with its placement attribution are emitted as
  before this change

#### Scenario: Test hooks are preserved

- **WHEN** `/notifications` renders notifications and alert subscriptions
- **THEN** each row exposes `data-testid="notification-item"` or `data-testid="alert-subscription"` and a
  `data-type` value as before
