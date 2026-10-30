# admin-moderation-ui Specification

## Purpose
Admin-only frontend pages for the listing moderation queue and the reports cases inbox, where admins decide queued listings and act on reports.

## Requirements

### Requirement: Admin moderation pages are admin-only

`/admin/listings` and `/admin/cases` SHALL redirect a request with no session principal or token to login through `redirectGuestToLogin` (keeping the path and the `tab` query for the return URL), SHALL render the `Result status="403"` titled "Cần tài khoản Quản trị" when the principal lacks the `admin` scope, and SHALL NOT call the gateway in either case. A gateway `PermissionDenied` SHALL render the same 403 and `Unauthenticated` SHALL redirect to login.

#### Scenario: Moderation UI: Anonymous visitor is redirected to login
- **WHEN** a request with no session cookie opens `/admin/listings` or `/admin/cases?tab=reports`
- **THEN** the page redirects to `/login` with a return URL for the same page and `ListModerationQueue` and `ListReports` are called zero times
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/listings/page.test.tsx` `redirects an anonymous visitor to login`

#### Scenario: Moderation UI: Anonymous browser visit
- **WHEN** a visitor with no session opens `/admin/listings` in the browser
- **THEN** the browser lands on `/login`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: Anonymous browser visit`

#### Scenario: Moderation UI: Non-admin sees the 403 result
- **WHEN** a signed-in buyer without the `admin` scope opens `/admin/listings` or `/admin/cases`
- **THEN** the page renders status 403 with the title "Cần tài khoản Quản trị" and a "Quay lại trang chủ" link, and no moderation or report RPC is called
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cases/page.test.tsx` `renders 403 for a non-admin`

#### Scenario: Moderation UI: A buyer opening the pages in the browser is refused
- **WHEN** a signed-in buyer opens `/admin/listings` in the browser
- **THEN** the page shows "Cần tài khoản Quản trị" and no queue table
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: A buyer opening the pages in the browser is refused`

#### Scenario: Moderation UI: Gateway refusal is shown as the same 403
- **WHEN** the principal carries the `admin` scope but the gateway answers `PermissionDenied`
- **THEN** the page renders the 403 result with the sub-text "Gateway từ chối quyền admin cho phiên đăng nhập này." and no table
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/listings/page.test.tsx` `renders 403 when the gateway denies`

#### Scenario: Moderation UI: Expired session is sent to login
- **WHEN** the gateway answers `Unauthenticated` for an admin principal
- **THEN** the page redirects to `/login`
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/listings/page.test.tsx` `redirects on gateway unauthenticated`

### Requirement: The queue lists pending listings oldest first

The queue view SHALL call `ListModerationQueue` once per render with `page.pageSize = 20` and the `cursor` from the URL, SHALL render the items in the returned order (oldest submission first) with title, seller id, price, submission time in Vietnam time (UTC+7, `YYYY-MM-DD HH:mm:ss`) and the screen flags, and SHALL render a "Trang sau" link only when the response `next_cursor` is non-empty. The link SHALL set `cursor` and keep `tab`. All values SHALL be rendered as text, never as HTML. An empty page SHALL show "Hàng chờ trống".

#### Scenario: Moderation UI: Rows show title, seller, price, submission time and flags
- **WHEN** the gateway returns one item titled "Thuốc lá lậu" by seller "s-1", price 120000, `submitted_at` 2026-10-05T17:00:00Z and flags `keyword:thuoc la`, `ai:review`
- **THEN** the page renders one row containing the title, "s-1", the formatted price, "2026-10-06 00:00:00", and the two flags
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerationQueueView.test.tsx` `renders one row with flags and UTC+7 time`

#### Scenario: Moderation UI: Server order is kept and page size is 20
- **WHEN** an admin opens the queue with no cursor and the gateway returns three items with `submitted_at` ascending
- **THEN** the client sends `page.pageSize = 20` and `page.cursor = ""` and the rows appear in the returned order
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/moderation.test.ts` `sends page size 20 and keeps order`

#### Scenario: Moderation UI: Next link carries the cursor
- **WHEN** the response has `next_cursor = "20"` on `/admin/cases?tab=queue`
- **THEN** the "Trang sau" link href is `/admin/cases?tab=queue&cursor=20`, and with an empty `next_cursor` the page has no "Trang sau" link
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerationQueueView.test.tsx` `next link keeps the tab and hides on the last page`

#### Scenario: Moderation UI: Empty queue
- **WHEN** the gateway returns zero items
- **THEN** the page shows "Hàng chờ trống" and no table rows
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerationQueueView.test.tsx` `shows the empty state`

#### Scenario: Moderation UI: Markup in a title or flag is shown as text
- **WHEN** an item title is `<script>alert(1)</script>`
- **THEN** the rendered DOM contains that string as text and zero `script` elements
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerationQueueView.test.tsx` `escapes titles and flags`

#### Scenario: Moderation UI: Queue backend unavailable
- **WHEN** the gateway answers `Unavailable` or the call times out after 5 s
- **THEN** the page shows "Không tải được hàng chờ kiểm duyệt. Vui lòng thử lại sau." and the tab strip stays usable
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/listings/page.test.tsx` `shows the unavailable state`

#### Scenario: Moderation UI: A flagged listing appears in the queue end to end
- **WHEN** a seller publishes a listing whose title contains the mock marker term and an admin opens `/admin/listings`
- **THEN** the queue lists that listing with a screen flag, and a clean listing published at the same time is not listed
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: A flagged listing appears in the queue end to end`

### Requirement: Admin decides a queued listing

Each queue row SHALL offer "Duyệt", "Từ chối" and "Khóa". "Duyệt" SHALL call `ModerateListing` with decision APPROVE. "Từ chối" and "Khóa" SHALL open a dialog that requires a reason code from the offered list (not `seller_enforcement`) and accepts an optional note of at most 500 characters, then call `ModerateListing` with decision REJECT or BAN. The service allows REJECT and BAN from `PENDING_REVIEW` or `PUBLISHED` and APPROVE only from `PENDING_REVIEW`; a REJECTED or BANNED listing cannot be reversed, so any refused transition is shown as the not-allowed message. The action SHALL send exactly one call per submit, SHALL block a second submit while pending, SHALL take no role or scope argument, SHALL revalidate the page on success so the row leaves the queue, and SHALL show a Vietnamese toast. A failure SHALL keep the row and SHALL never show `err.message`.

#### Scenario: Moderation UI: Approve publishes and removes the row
- **WHEN** an admin clicks "Duyệt" on a queued listing and the gateway answers OK
- **THEN** one `ModerateListing` call with decision APPROVE is sent, the toast "Đã duyệt tin đăng." appears and the row is gone after revalidation
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `approve sends one call and revalidates`

#### Scenario: Moderation UI: Reject needs a reason code
- **WHEN** an admin opens the "Từ chối" dialog and submits with no reason code
- **THEN** the submit is blocked, no gateway call is made and the dialog shows "Vui lòng chọn lý do."
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerateDialog.test.tsx` `blocks submit without a reason code`

#### Scenario: Moderation UI: Reject sends reason and note
- **WHEN** an admin rejects with reason `counterfeit` and the note "Ảnh sản phẩm là hàng nhái"
- **THEN** the gateway receives decision REJECT, `reason_code = "counterfeit"` and that note, and the toast "Đã từ chối tin đăng." appears
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `reject sends reason and note`

#### Scenario: Moderation UI: Moderation note length boundary
- **WHEN** an admin types a note of 501 characters
- **THEN** the counter shows 500/500, the input accepts no more than 500 characters and the action rejects a 501-character note locally with no gateway call
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerateDialog.test.tsx` `caps the note at 500 characters`

#### Scenario: Moderation UI: Ban removes the listing from the queue
- **WHEN** an admin bans a queued listing with reason `prohibited_item`
- **THEN** one `ModerateListing` call with decision BAN is sent, the toast "Đã khóa tin đăng." appears and the row leaves the queue
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `ban sends one call`

#### Scenario: Moderation UI: Moderation double submit is blocked
- **WHEN** an admin double-clicks the confirm button of a dialog while the first call is pending
- **THEN** exactly one `ModerateListing` call is made and the button shows its pending state
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ModerateDialog.test.tsx` `pending state blocks a double submit`

#### Scenario: Moderation UI: Audit unavailable leaves the row
- **WHEN** the gateway answers `Unavailable` because team-audit is down
- **THEN** the dialog shows "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.", the row stays in the queue and no toast of success appears
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `maps unavailable to the audit message`

#### Scenario: Moderation UI: Not-allowed transition is explained
- **WHEN** the gateway answers `FailedPrecondition` for the decision
- **THEN** the dialog shows "Trạng thái hiện tại của tin đăng không cho phép thao tác này." and the row is not removed by the client
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `maps failed precondition`

#### Scenario: Moderation UI: Listing is gone
- **WHEN** the gateway answers `NotFound`
- **THEN** the dialog shows "Tin đăng không còn tồn tại."
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `maps not found`

#### Scenario: Moderation UI: A forged action by a non-admin is refused
- **WHEN** a non-admin session invokes the moderation server action directly and the gateway answers `PermissionDenied`
- **THEN** the action returns "Bạn không có quyền thực hiện thao tác này." and the listing status is unchanged
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/moderationActions.test.ts` `maps permission denied`

#### Scenario: Moderation UI: Admin approves a flagged listing end to end
- **WHEN** an admin clicks "Duyệt" on a flagged listing in `/admin/listings`
- **THEN** the row disappears and the listing opens on its product page for an anonymous visitor
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: Admin approves a flagged listing end to end`

#### Scenario: Moderation UI: Admin rejects a flagged listing and the seller sees the reason
- **WHEN** an admin rejects a flagged listing with reason `counterfeit` and a note
- **THEN** the row disappears, the listing is NOT_FOUND for an anonymous visitor, and the seller center shows the status "Bị từ chối" with the reason label and the note
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: Admin rejects a flagged listing and the seller sees the reason`

#### Scenario: Moderation UI: Moderation fails closed when audit is down
- **WHEN** team-audit is stopped and an admin approves a flagged listing in the browser
- **THEN** the dialog or banner shows the audit message and, after team-audit restarts, the listing is still in the queue
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature` `Moderation UI: Moderation fails closed when audit is down`

### Requirement: The cases inbox has tabs and a report list

`/admin/cases` SHALL render a tab strip built from a list of tabs and SHALL show the tab named by `?tab=` (`reports` or `queue` in this change; any other value SHALL show `reports`). The `queue` tab SHALL render the same view as `/admin/listings`. The `reports` tab SHALL call `ListReports` once per render with the status filter from `?status=` (`OPEN`, `ACTIONED` or `DISMISSED`; default and fallback `OPEN`), `page.pageSize = 20` and the URL `cursor`, SHALL render reports in the returned order (oldest first) with listing id, reason label, note, reporter id, created time (UTC+7) and status, and SHALL render "Trang sau" only when `next_cursor` is non-empty. An empty page SHALL show "Không có báo cáo nào". A report whose listing no longer exists (the moderation call answers `NotFound`) SHALL show "Tin đăng đã bị gỡ".

#### Scenario: Moderation UI: Default tab is reports with status OPEN
- **WHEN** an admin opens `/admin/cases` with no query
- **THEN** the "Báo cáo" tab is selected and `ListReports` is called with status OPEN and `pageSize = 20`
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cases/page.test.tsx` `defaults to the reports tab with OPEN`

#### Scenario: Moderation UI: Unknown tab or status falls back
- **WHEN** an admin opens `/admin/cases?tab=evil&status=bogus`
- **THEN** the reports tab shows with status filter OPEN and no error
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cases/page.test.tsx` `falls back on unknown tab and status`

#### Scenario: Moderation UI: Status filter reloads the list
- **WHEN** an admin selects "Đã xử lý" in the status filter
- **THEN** the URL becomes `/admin/cases?tab=reports&status=ACTIONED` and `ListReports` is called with status ACTIONED
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ReportsView.test.tsx` `status filter links keep the tab`

#### Scenario: Moderation UI: Report row shows reporter to the admin
- **WHEN** the gateway returns a report for listing "l-1" with reason `scam`, note "Giá quá rẻ", reporter "u-9", created 2026-10-05T17:00:00Z
- **THEN** the row shows "l-1", "Lừa đảo", "Giá quá rẻ", "u-9" and "2026-10-06 00:00:00"
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ReportsView.test.tsx` `renders one report row`

#### Scenario: Moderation UI: Report note markup is shown as text
- **WHEN** a report note is `<img src=x onerror=alert(1)>`
- **THEN** the DOM contains it as text and zero `img` elements from the note
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ReportsView.test.tsx` `escapes report notes`

#### Scenario: Moderation UI: Empty report list
- **WHEN** the gateway returns zero reports
- **THEN** the page shows "Không có báo cáo nào"
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ReportsView.test.tsx` `shows the empty state`

#### Scenario: Moderation UI: Reports backend unavailable
- **WHEN** `ListReports` answers `Unavailable`
- **THEN** the reports tab shows "Không tải được danh sách báo cáo. Vui lòng thử lại sau." and the tab strip stays usable
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cases/page.test.tsx` `shows the reports unavailable state`

#### Scenario: Moderation UI: Removed listing is labelled
- **WHEN** an admin starts "Xử lý tin đăng" on a report and `ModerateListing` answers `NotFound`
- **THEN** the row shows "Tin đăng đã bị gỡ" and the close-report control stays available
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/ReportsView.test.tsx` `labels a removed listing`

### Requirement: Admin acts on a report and closes it

A report row SHALL offer "Xử lý tin đăng" (the reject / ban dialog of the queue, applied to the reported listing through `ModerateListing`) and "Đóng báo cáo". The close dialog SHALL require an outcome (`ACTIONED` or `DISMISSED`) and a trimmed note of 1 to 500 characters, SHALL call `ResolveReport` once, SHALL block a second submit while pending, and SHALL revalidate `/admin/cases` so a closed report leaves the `OPEN` filter. Closing SHALL be independent of moderating: neither call implies the other.

#### Scenario: Moderation UI: Close a report as actioned
- **WHEN** an admin closes a report with outcome "Đã xử lý" and the note "Đã khóa tin đăng"
- **THEN** one `ResolveReport` call with status ACTIONED and that note is sent, the toast "Đã đóng báo cáo." appears and the report leaves the OPEN list
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `close sends one call and revalidates`

#### Scenario: Moderation UI: Close needs a note
- **WHEN** an admin submits the close dialog with an empty or whitespace note
- **THEN** the submit is blocked, no call is made and the dialog shows "Vui lòng nhập ghi chú xử lý."
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/CloseReportDialog.test.tsx` `blocks an empty note`

#### Scenario: Moderation UI: Close note length boundary
- **WHEN** the note has 500 characters, and then 501 characters
- **THEN** the 500-character note is sent and the 501-character note is blocked locally with no gateway call
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `validates note length 1 to 500`

#### Scenario: Moderation UI: Dismiss a report
- **WHEN** an admin closes a report with outcome "Bác bỏ" and a note
- **THEN** one `ResolveReport` call with status DISMISSED is sent and the report appears under the "Đã bác bỏ" filter
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `dismiss sends DISMISSED`

#### Scenario: Moderation UI: Close fails closed when audit is down
- **WHEN** `ResolveReport` answers `Unavailable`
- **THEN** the dialog shows the audit message and the report stays OPEN
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `maps unavailable to the audit message`

#### Scenario: Moderation UI: Already-closed report
- **WHEN** `ResolveReport` answers `FailedPrecondition`
- **THEN** the dialog shows "Báo cáo này đã được xử lý." and the list is revalidated
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `maps failed precondition`

#### Scenario: Moderation UI: A forged close by a non-admin is refused
- **WHEN** a non-admin session invokes the close server action and the gateway answers `PermissionDenied`
- **THEN** the action returns "Bạn không có quyền thực hiện thao tác này." and the report is unchanged
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/reportActions.test.ts` `maps permission denied`

#### Scenario: Moderation UI: Admin bans a reported listing and closes the report
- **WHEN** a buyer reports a published listing, an admin opens `/admin/cases`, uses "Xử lý tin đăng" with decision "Khóa" and then closes the report as "Đã xử lý" with a note
- **THEN** the listing is NOT_FOUND for an anonymous visitor and the report is listed under "Đã xử lý" and no longer under "Đang mở"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: Admin bans a reported listing and closes the report`

#### Scenario: Moderation UI: Admin dismisses a report
- **WHEN** an admin closes an open report as "Bác bỏ" with a note
- **THEN** the report moves from "Đang mở" to "Đã bác bỏ" and the listing stays visible to an anonymous visitor
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: Admin dismisses a report`

#### Scenario: Moderation UI: Closing a report fails closed when audit is down
- **WHEN** team-audit is stopped and an admin closes a report in the browser
- **THEN** the dialog shows the audit message and the report is still OPEN after team-audit restarts
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: Closing a report fails closed when audit is down`

### Requirement: Admin entry links

The cockpit page and the admin footer list in `ConsumerShell` SHALL link to `/admin/listings` ("Kiểm duyệt tin đăng") and `/admin/cases` ("Hộp thư vụ việc"), using internal `next/link`.

#### Scenario: Moderation UI: Admin reaches the pages from the cockpit
- **WHEN** an admin opens `/admin/cockpit` and clicks "Hộp thư vụ việc"
- **THEN** the browser lands on `/admin/cases` and the "Báo cáo" tab is selected
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: Admin reaches the pages from the cockpit`

#### Scenario: Moderation UI: Cockpit shows both links
- **WHEN** an admin renders the cockpit view
- **THEN** it contains links to `/admin/listings` and `/admin/cases`
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/CockpitView.test.tsx` `shows the moderation links`

#### Scenario: Moderation UI: A non-admin following an admin link is refused
- **WHEN** a signed-in buyer follows the footer link "Hộp thư vụ việc" to `/admin/cases`
- **THEN** the page shows "Cần tài khoản Quản trị" and no report row, and `ListReports` is called zero times
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: A non-admin following an admin link is refused`
