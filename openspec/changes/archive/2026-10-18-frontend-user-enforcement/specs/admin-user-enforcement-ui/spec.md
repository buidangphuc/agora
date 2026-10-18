## ADDED Requirements

### Requirement: The users page is admin-only

`/admin/users` SHALL call `redirectGuestToLogin("/admin/users", searchParams)` when there is no session principal or token, SHALL render the `Result status="403"` titled "Cần tài khoản Quản trị" when the principal lacks the `admin` scope, and SHALL NOT call the gateway in either case. When the gateway answers `PermissionDenied` the page SHALL render the same 403 result, and when it answers `Unauthenticated` the page SHALL call `redirectGuestToLogin`. The page metadata SHALL set `referrer` to `no-referrer`.

#### Scenario: Users UI: Anonymous visitor is redirected to login
- **WHEN** a request with no session cookie opens `/admin/users?query=an`
- **THEN** the page calls `redirectGuestToLogin` with path `/admin/users` and search params `query=an`, and `ListUsers` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `redirects an anonymous visitor to login`

#### Scenario: Users UI: Anonymous browser visit
- **WHEN** a visitor with no session opens `/admin/users` in the browser
- **THEN** the browser lands on `/login` with a `returnUrl` for `/admin/users`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `An anonymous visitor is sent to login`

#### Scenario: Users UI: Non-admin sees the 403 result
- **WHEN** a signed-in buyer without the `admin` scope opens `/admin/users`
- **THEN** the page renders status 403 with the title "Cần tài khoản Quản trị" and a "Quay lại trang chủ" link, and `ListUsers` is called zero times
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `renders 403 for a non-admin`

#### Scenario: Users UI: A buyer opening the page in the browser is refused
- **WHEN** a signed-in buyer opens `/admin/users` in the browser
- **THEN** the page shows "Cần tài khoản Quản trị" and no user table
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `A buyer cannot open the users page`

#### Scenario: Users UI: Gateway refusal is shown as the same 403
- **WHEN** the principal carries the `admin` scope but the gateway answers `PermissionDenied`
- **THEN** the page renders the 403 result with the sub-text "Gateway từ chối quyền admin cho phiên đăng nhập này." and no table
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `renders 403 when the gateway denies`

#### Scenario: Users UI: Expired session is sent to login
- **WHEN** the gateway answers `Unauthenticated` for an admin principal
- **THEN** the page calls `redirectGuestToLogin("/admin/users", ...)`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `redirects on gateway unauthenticated`

#### Scenario: Users UI: Login guard test still passes
- **WHEN** the Q1 guard test scans `src/app` after this change
- **THEN** it reports zero bare `redirect("/login...")` calls
- **VERIFIED BY**: `team-frontend/src/app/guards.test.ts` `no bare redirect to login`

#### Scenario: Users UI: The users page sends no referrer
- **WHEN** the page module's `metadata` is read
- **THEN** `metadata.referrer` equals `no-referrer`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `metadata sets no-referrer`

### Requirement: Admin finds users by username or id

The page SHALL call `AccountAdminService.ListUsers` once per render with `query` (trimmed and cut to at most 64 bytes in UTF-8 at a character boundary), `status` and `page.cursor` from the URL and `page.pageSize = 20`, SHALL keep the server order (username ascending), and SHALL render exactly the columns Tên đăng nhập, ID, Vai trò, Trạng thái, Lý do, Hết hạn lúc and Thao tác (`created_at` is not shown). `status_until` SHALL be shown as `YYYY-MM-DD HH:mm` in UTC+07:00. It SHALL render a "Trang sau" link only when the response `next_cursor` is non-empty, keeping every current filter. Values SHALL be rendered as text, never as HTML.

#### Scenario: Users UI: Rows show the user columns
- **WHEN** the gateway returns one user "an01" with roles seller and buyer, status SUSPENDED, reason `fraud`, until 2026-10-20T10:00:00Z
- **THEN** the page renders one row with "an01", the id, "seller, buyer", "Tạm khóa", "fraud" and "2026-10-20 17:00"
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `renders one row with status and local until`

#### Scenario: Users UI: Search is forwarded as typed fields
- **WHEN** an admin opens `/admin/users?query=an01&status=suspended&cursor=c2`
- **THEN** `ListUsers` receives `query = "an01"`, `status = USER_STATUS_SUSPENDED`, `page.cursor = "c2"` and `page.pageSize = 20`
- **VERIFIED BY**: `team-frontend/src/lib/gateway/users.test.ts` `forwards query, status, cursor and page size 20`

#### Scenario: Users UI: Oversized or unknown filter values are bounded
- **WHEN** the URL carries, in turn, a 300-character ASCII `query`, a `query` of 40 "ư" (2 bytes each in UTF-8), an array-valued `query`, and `status=banana`
- **THEN** the ASCII query is cut to 64 characters, the "ư" query is cut to 32 characters (64 bytes, no split character), the first array value is used, and the status filter is not sent
- **VERIFIED BY**: `team-frontend/src/features/admin/userFilters.test.ts` `caps query at 64 UTF-8 bytes, takes first array value, drops unknown status`

#### Scenario: Users UI: Next page link carries cursor and filters
- **WHEN** the response has `next_cursor` "c3" and the URL has `query=an`
- **THEN** the "Trang sau" link href contains `cursor=c3` and `query=an`
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `next link keeps filters`

#### Scenario: Users UI: Last page has no next link
- **WHEN** the response `next_cursor` is empty
- **THEN** the page renders no "Trang sau" link
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `no next link on the last page`

#### Scenario: Users UI: Markup in a username is shown as text
- **WHEN** a returned username is `<img src=x onerror=alert(1)>`
- **THEN** the cell contains that literal text and the rendered tree contains zero `img` elements
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `renders markup as text`

#### Scenario: Users UI: No matching user
- **WHEN** `ListUsers` returns zero users
- **THEN** the page shows "Không tìm thấy người dùng nào" and no table rows
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `shows the empty state`

#### Scenario: Users UI: Search by a principal that lost the admin scope is denied
- **WHEN** the cookie still carries the `admin` scope but the gateway answers `PermissionDenied` to a `ListUsers` search
- **THEN** the page renders the 403 result and zero user rows
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/users/page.test.tsx` `renders 403 and no rows on a denied search`

#### Scenario: Users UI: Identity unavailable
- **WHEN** `ListUsers` fails with `Unavailable` or times out after 5 s
- **THEN** the page shows "Không tải được danh sách người dùng. Vui lòng thử lại sau." and keeps the search form usable
- **VERIFIED BY**: `team-frontend/src/lib/gateway/users.test.ts` `maps unavailable and timeout`

#### Scenario: Users UI: Admin finds a user by username in the browser
- **WHEN** an admin searches `/admin/users` for the username of a freshly registered buyer
- **THEN** the table shows that buyer with status "Hoạt động"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Admin finds a user by username`

### Requirement: Admin changes a user's status

The row action form SHALL send `AccountAdminService.SetUserStatus` with `user_id`, the chosen `status`, a non-empty `reason_code`, the optional `note` (at most 500 characters) and, only for SUSPENDED, `until` read as UTC+07:00. The server action SHALL re-check the session and the `admin` scope before any gateway call, SHALL refuse locally (no gateway call) when the reason is empty, when SUSPENDED has no end time or an end time not in the future, or when the note is over 500 characters, and SHALL NOT send `until` for another status. On success it SHALL show "Đã cập nhật trạng thái; đã đăng xuất N phiên." with N = `sessions_revoked` and refresh the row. The page SHALL render no action form on the signed-in admin's own row or on a row whose roles include `admin`.

#### Scenario: Users UI: Suspend with an end time
- **WHEN** an admin submits SUSPENDED, reason `fraud`, note "bán hàng giả", end time 2026-10-20 17:00 for a buyer
- **THEN** `SetUserStatus` is called once with `status = USER_STATUS_SUSPENDED`, `reason_code = "fraud"`, `until = 2026-10-20T10:00:00Z`, and `revalidatePath("/admin/users")` is called once
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `suspend sends until in UTC+7`

#### Scenario: Users UI: Ban and restrict send no end time
- **WHEN** an admin submits BANNED, then RESTRICTED, each with a reason and a leftover end-time value in the form
- **THEN** both `SetUserStatus` calls carry no `until`
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `until is sent only for suspended`

#### Scenario: Users UI: Success shows how many sessions ended
- **WHEN** `SetUserStatus` answers `sessions_revoked = 2`
- **THEN** the page shows "Đã cập nhật trạng thái; đã đăng xuất 2 phiên."
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `reports sessions revoked`

#### Scenario: Users UI: Reinstating shows zero sessions ended
- **WHEN** an admin sets a banned user to ACTIVE with a reason and `SetUserStatus` answers `sessions_revoked = 0`
- **THEN** the action result message is "Đã cập nhật trạng thái; đã đăng xuất 0 phiên." and no `until` is sent
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `reinstating reports zero sessions`

#### Scenario: Users UI: Invalid input or unknown user from the server is reported
- **WHEN** `SetUserStatus` answers `InvalidArgument` with the raw message "reason_code too long", then `NotFound`
- **THEN** the action returns the error "Dữ liệu không hợp lệ; trạng thái không thay đổi.", then "Không tìm thấy người dùng này; trạng thái không thay đổi.", neither contains the raw message, and neither returns the success message
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `maps invalid and not found to fixed messages`

#### Scenario: Users UI: Missing reason is refused locally
- **WHEN** an admin submits a status change with no reason code
- **THEN** the form shows "Vui lòng chọn mã lý do." and `SetUserStatus` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `refuses an empty reason`

#### Scenario: Users UI: Suspension without a future end time is refused locally
- **WHEN** an admin submits SUSPENDED with no end time, then with an end time one minute in the past
- **THEN** both attempts show "Chọn thời điểm kết thúc tạm khóa trong tương lai." and `SetUserStatus` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `refuses missing or past until`

#### Scenario: Users UI: Note of 501 characters is refused locally
- **WHEN** an admin submits a note of 501 characters
- **THEN** the form shows "Ghi chú tối đa 500 ký tự." and `SetUserStatus` is called zero times; a note of exactly 500 characters is sent
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `note boundary 500`

#### Scenario: Users UI: Non-admin cannot run the action
- **WHEN** the server action runs with a session whose scopes lack `admin`, or with no session
- **THEN** it returns an error result (no session: a login redirect) and `SetUserStatus` is called zero times
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `rechecks admin and session`

#### Scenario: Users UI: Own row and admin rows have no action form
- **WHEN** the list contains the signed-in admin and another user with the `admin` role
- **THEN** neither row renders a status form, and other rows do
- **VERIFIED BY**: `team-frontend/src/features/admin/UsersView.test.tsx` `hides actions for self and admins`

#### Scenario: Users UI: Server refuses a self or admin target
- **WHEN** `SetUserStatus` answers `FAILED_PRECONDITION`
- **THEN** the page shows "Không thể thay đổi trạng thái của chính bạn hoặc của một quản trị viên khác." and the row is unchanged
- **VERIFIED BY**: `team-frontend/src/lib/gateway/users.test.ts` `maps failed precondition`

#### Scenario: Users UI: Audit service down leaves the user unchanged
- **WHEN** `SetUserStatus` answers `Unavailable` because team-audit is down
- **THEN** the page shows "Hệ thống chưa ghi nhận được thao tác; trạng thái không thay đổi. Vui lòng thử lại sau." and never shows the success message
- **VERIFIED BY**: `team-frontend/src/lib/gateway/users.test.ts` `maps unavailable on set status`

#### Scenario: Users UI: Expired session during the action is sent to login
- **WHEN** `SetUserStatus` answers `Unauthenticated`
- **THEN** the action calls `redirectGuestToLogin("/admin/users")`
- **VERIFIED BY**: `team-frontend/src/features/admin/userActions.test.ts` `redirects on unauthenticated`

#### Scenario: Users UI: Admin suspends a buyer from the users page
- **WHEN** an admin finds a fresh buyer and submits SUSPENDED with reason `fraud` and an end time one hour ahead
- **THEN** the page shows the success message and the buyer's row reads "Tạm khóa" with that end time
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Admin suspends a buyer from the users page`

#### Scenario: Users UI: Admin bans then reinstates a buyer
- **WHEN** an admin bans a fresh buyer with reason `abuse`, then sets the same buyer to "Hoạt động" with a reason
- **THEN** the row reads "Cấm" after the first change and "Hoạt động" after the second
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Admin bans then reinstates a buyer`

#### Scenario: Users UI: Admin has no controls on their own row
- **WHEN** the admin searches for their own username
- **THEN** the row has no status form
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Admin cannot change their own status`

#### Scenario: Users UI: Audit down in the browser leaves the user active
- **WHEN** team-audit is stopped and an admin suspends a fresh buyer
- **THEN** the page shows "Hệ thống chưa ghi nhận được thao tác; trạng thái không thay đổi. Vui lòng thử lại sau." and the buyer can still log in
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Audit outage leaves the user unchanged`

### Requirement: Cockpit links to the users page for admins

The cockpit page SHALL show the link "Quản lý người dùng" to `/admin/users` only when it renders for an admin principal; a non-admin principal gets the 403 result and no link.

#### Scenario: Users UI: Cockpit shows the users link
- **WHEN** an admin opens `/admin/cockpit`
- **THEN** the page contains one link "Quản lý người dùng" with href `/admin/users`
- **VERIFIED BY**: `team-frontend/src/features/admin/CockpitView.test.tsx` `shows the users link`

#### Scenario: Users UI: Non-admin cockpit visitor sees no users link
- **WHEN** a signed-in buyer opens `/admin/cockpit`
- **THEN** the page does not contain a "Quản lý người dùng" link and the 403 result is shown
- **VERIFIED BY**: `team-frontend/src/app/(shop)/admin/cockpit/page.test.tsx` `non-admin sees no users link`
