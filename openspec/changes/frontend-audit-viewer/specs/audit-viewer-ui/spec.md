## ADDED Requirements

### Requirement: The audit page is admin-only

`/admin/audit` SHALL redirect to `/login` when there is no session principal or token, SHALL render the same `Result status="403"` titled "Cần tài khoản Quản trị" as `/admin/cockpit` when the principal lacks the `admin` scope, and SHALL NOT call the gateway in either case. When the gateway answers `PermissionDenied` the page SHALL render the same 403 result, and when it answers `Unauthenticated` the page SHALL redirect to `/login`.

#### Scenario: Anonymous visitor is redirected to login
- **WHEN** a request with no session cookie opens `/admin/audit`
- **THEN** the page redirects to `/login` and `QueryAuditLog` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `redirects an anonymous visitor to login`

#### Scenario: Anonymous browser visit
- **WHEN** a visitor with no session opens `/admin/audit` in the browser
- **THEN** the browser lands on `/login`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `An anonymous visitor is sent to login`

#### Scenario: Non-admin sees the 403 result
- **WHEN** a signed-in buyer without the `admin` scope opens `/admin/audit`
- **THEN** the page renders status 403 with the title "Cần tài khoản Quản trị" and a "Quay lại trang chủ" link, and `QueryAuditLog` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `renders 403 for a non-admin`

#### Scenario: Gateway refusal is shown as the same 403
- **WHEN** the principal carries the `admin` scope but the gateway answers `PermissionDenied`
- **THEN** the page renders the 403 result with the sub-text "Gateway từ chối quyền admin cho phiên đăng nhập này." and no table
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `renders 403 when the gateway denies`

#### Scenario: Expired session is sent to login
- **WHEN** the gateway answers `Unauthenticated` for an admin principal
- **THEN** the page redirects to `/login`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `redirects on gateway unauthenticated`

#### Scenario: A buyer opening the page in the browser is refused
- **WHEN** a signed-in buyer opens `/admin/audit` in the browser
- **THEN** the page shows "Cần tài khoản Quản trị" and no audit table
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `A buyer cannot open the audit page`

### Requirement: Events are listed newest first, 20 per page

The page SHALL call `AuditService.QueryAuditLog` once per render with `page.pageSize = 20` and the `cursor` from the URL, SHALL render the events in the order returned (newest first) in a table with the columns Thời gian, Người thực hiện, Hành động, Đối tượng (`target_type/target_id`) and Chi tiết (metadata as `key=value` pairs with keys sorted), and SHALL render a "Trang sau" link only when the response `next_cursor` is non-empty. The link SHALL keep every current filter and set `cursor`. Event values SHALL be rendered as text, never as HTML.

#### Scenario: Rows show the five columns
- **WHEN** the gateway returns one event with actor "u-1", action "kyc.review", target `kyc_submission/s-9`, metadata `{user_id: "u-7", decision: "approve"}`
- **THEN** the page renders one row containing "u-1", "kyc.review", "kyc_submission/s-9", the formatted time, and the metadata text "decision=approve, user_id=u-7"
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `renders one row with sorted metadata`

#### Scenario: Server order is kept
- **WHEN** the gateway returns three events with `created_at` descending
- **THEN** the table rows appear in that same order
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `keeps the server order`

#### Scenario: Page size is 20
- **WHEN** an admin opens `/admin/audit` with no cursor
- **THEN** the gateway client sends `page.pageSize = 20` and `page.cursor = ""`
- **VERIFIED BY**: `team-frontend/src/lib/gateway/audit.test.ts` `sends page size 20 and the cursor`

#### Scenario: Time is shown in Vietnam time
- **WHEN** an event has `created_at` 2026-10-05T17:00:00Z
- **THEN** the Thời gian cell shows "2026-10-06 00:00:00"
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `formats time in UTC+7`

#### Scenario: Next page link carries cursor and filters
- **WHEN** the response has `next_cursor = "20"` and the URL has `target_id=o-1&action=order.force_fail`
- **THEN** the "Trang sau" link href is `/admin/audit?target_id=o-1&action=order.force_fail&cursor=20`
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `next link keeps filters`

#### Scenario: Last page has no next link
- **WHEN** the response `next_cursor` is empty
- **THEN** the page contains no "Trang sau" link
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `hides next link on last page`

#### Scenario: Markup in metadata is shown as text
- **WHEN** an event metadata value is `<script>alert(1)</script>`
- **THEN** the rendered DOM contains that string as text and contains zero `script` elements
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `escapes metadata values`

#### Scenario: Seeded events paginate end to end
- **WHEN** an admin has 25 events seeded with `seed_events` at distinct whole-second `created_at` values for one target id and opens `/admin/audit?target_id=<id>`, then clicks "Trang sau"
- **THEN** page 1 shows 20 rows with the newest event first and page 2 shows the remaining 5 rows and no "Trang sau" link
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Admin pages through 25 events`

### Requirement: Filters live in the URL and map to QueryAuditLog

The page SHALL read `target_type`, `target_id`, `actor_id`, `action`, `from` and `to` from the URL query string, trim them, ignore empty values, cap `target_type`, `target_id`, `actor_id` and `action` at 128 characters and `cursor` at 256, and send only the non-empty ones as `targetType`, `targetId`, `actorId`, `action`, `createdAfter`, `createdBefore`. `from` and `to` SHALL be `YYYY-MM-DD` days in UTC+07:00, both inclusive: `createdAfter` is `from` 00:00 +07:00 and `createdBefore` is the day after `to` at 00:00 +07:00. The filter form SHALL be a GET form pre-filled from the URL, with a "Lọc" button and a "Xóa bộ lọc" link to `/admin/audit`.

#### Scenario: All filters are forwarded
- **WHEN** the URL is `?target_type=order&target_id=o-1&actor_id=a-1&action=order.force_fail&from=2026-10-01&to=2026-10-02`
- **THEN** the request has targetType "order", targetId "o-1", actorId "a-1", action "order.force_fail", createdAfter 2026-09-30T17:00:00Z and createdBefore 2026-10-02T17:00:00Z
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `maps url params to request filters`

#### Scenario: Empty and whitespace filters are omitted
- **WHEN** the URL is `?target_id=%20%20&action=`
- **THEN** the request carries no targetId, no action and no time bound
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `omits blank values`

#### Scenario: Same day range is valid and inclusive
- **WHEN** `from` and `to` are both `2026-10-05`
- **THEN** createdAfter is 2026-10-04T17:00:00Z and createdBefore is 2026-10-05T17:00:00Z
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `same day is inclusive`

#### Scenario: Oversized value is capped
- **WHEN** `actor_id` is 500 characters long and `cursor` is 600 characters long
- **THEN** the request actorId is its first 128 characters and the cursor is its first 256 characters
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `caps long values`

#### Scenario: Only a start date is given
- **WHEN** the URL is `?from=2026-10-01` with no `to`
- **THEN** createdAfter is 2026-09-30T17:00:00Z and the request carries no createdBefore
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `from only sets a lower bound`

#### Scenario: Only an end date is given
- **WHEN** the URL is `?to=2026-10-02` with no `from`
- **THEN** createdBefore is 2026-10-02T17:00:00Z and the request carries no createdAfter
- **VERIFIED BY**: `team-frontend/src/features/admin/auditFilters.test.ts` `to only sets an upper bound`

#### Scenario: Unparseable date is refused locally
- **WHEN** `from` is `2026-13-45` or `abc`
- **THEN** the page shows "Ngày không hợp lệ. Vui lòng chọn lại." and `QueryAuditLog` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `shows a local error for a bad date`

#### Scenario: Form is pre-filled from the URL
- **WHEN** the page renders with `?actor_id=a-1&from=2026-10-01`
- **THEN** the "Người thực hiện (ID)" input has value "a-1", the "Từ ngày" input has value "2026-10-01", and the form method is GET
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `prefills filters from the url`

#### Scenario: Filtering by target narrows the list
- **WHEN** an admin has seeded events for two different order ids and opens `/admin/audit?target_type=order&target_id=<first id>`
- **THEN** every row shows the first order id and no row shows the second
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Admin filters events by target and action`

#### Scenario: Filtering by action and actor
- **WHEN** an admin seeds events with a per-run unique actor id with actions "kyc.review" and "order.force_fail" and submits the form with the action "kyc.review" and the actor id of the seeded actor
- **THEN** only rows with action "kyc.review" and that actor are listed
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Admin filters events by target and action`

#### Scenario: Day edge in Vietnam time
- **WHEN** an admin has events at 2026-10-05T16:59:59Z and 2026-10-05T17:00:00Z for one target and filters with `from` and `to` both 2026-10-05
- **THEN** the list shows the 16:59:59Z event and does not show the 17:00:00Z event
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Day edge follows Vietnam time`

#### Scenario: Date range excludes events outside it
- **WHEN** an admin filters with `from` and `to` both set to a day with no seeded events
- **THEN** the page shows "Không có sự kiện nào"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Admin filters events by date range`

#### Scenario: Filters never reach the gateway for a non-admin
- **WHEN** a signed-in buyer opens `/admin/audit?actor_id=a-1`
- **THEN** the page renders the 403 result and `QueryAuditLog` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `does not query for a non-admin with filters`

### Requirement: Empty, invalid-range and error states are shown in Vietnamese

The page SHALL render "Không có sự kiện nào" when the response has zero events, SHALL map a gateway `InvalidArgument` to "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu." in an alert above the form (form stays usable, no table), and SHALL map `Unavailable` or any other failure to "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau.". The raw gateway message SHALL NOT be shown.

#### Scenario: No matching events
- **WHEN** the gateway returns zero events
- **THEN** the page shows "Không có sự kiện nào" and no table rows and no "Trang sau" link
- **VERIFIED BY**: `team-frontend/src/features/admin/AuditLogView.test.tsx` `shows the empty state`

#### Scenario: End date before start date
- **WHEN** the URL has `from=2026-10-05&to=2026-10-01` and the gateway answers `InvalidArgument`
- **THEN** the page shows "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu." and no table
- **VERIFIED BY**: `team-frontend/src/lib/gateway/audit.test.ts` `maps InvalidArgument to invalid_range`; `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `shows the invalid-range message and no table`

#### Scenario: End date before start date in the browser
- **WHEN** an admin submits the form with "Từ ngày" 2026-10-05 and "Đến ngày" 2026-10-01
- **THEN** the page shows the invalid-range message and the form keeps both dates
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Reversed date range shows an error`

#### Scenario: Server-rejected cursor
- **WHEN** the URL carries a cursor the gateway rejects with `InvalidArgument` while no `from` or `to` is set
- **THEN** the page shows "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau." and no table
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `maps a rejected cursor to unavailable`

#### Scenario: team-audit unavailable
- **WHEN** the gateway call fails with `Unavailable` or times out
- **THEN** the page shows "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau." and the raw error text appears nowhere in the HTML
- **VERIFIED BY**: `team-frontend/src/lib/gateway/audit.test.ts` `maps Unavailable and unknown errors to unavailable`; `team-frontend/src/app/(shop)/admin/audit/page.test.tsx` `shows the unavailable message, no raw error, no table`

### Requirement: The cockpit links to the audit page

The cockpit page SHALL show a link "Nhật ký kiểm toán" to `/admin/audit` for admins.

#### Scenario: Cockpit shows the audit link
- **WHEN** an admin opens `/admin/cockpit`
- **THEN** the page contains a link named "Nhật ký kiểm toán" whose href is `/admin/audit`
- **VERIFIED BY**: `team-frontend/src/features/admin/CockpitView.test.tsx` `links to the audit log`

#### Scenario: Following the cockpit link
- **WHEN** an admin clicks "Nhật ký kiểm toán" on the cockpit
- **THEN** the browser lands on `/admin/audit` showing the heading "Nhật ký kiểm toán"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature` `Admin reaches the audit page from the cockpit`

#### Scenario: Non-admin cockpit visitor sees no audit link
- **WHEN** a signed-in buyer without the `admin` scope opens `/admin/cockpit`
- **THEN** the page renders the 403 result and contains no link to `/admin/audit`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cockpit/page.test.tsx` `403 has no audit link`
