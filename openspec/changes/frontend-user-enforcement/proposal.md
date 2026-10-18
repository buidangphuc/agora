## Why

initiative: q6-trust-enforcement

Initiative q6-trust-enforcement lets an admin restrict, suspend or ban a user (`AccountAdminService.SetUserStatus`, shipped by identity-user-enforcement) but an operator has no screen for it: today the only way is a gRPC call. This slice adds the admin page `/admin/users` (find a user, see the status, change it) and makes the login and session-refresh paths tell a suspended or banned user what happened instead of a generic error or a silent retry loop.

depends_on: identity-user-enforcement (the `AccountAdminService` RPCs, the `PERMISSION_DENIED` on Login/RefreshSession of a suspended or banned user, and the gateway registration + `adminScope` entries for both RPCs, all done in that slice's team-gateway task). Proto copies in `team-frontend/proto` come from q6-trust-contract; this slice never edits proto copies, it only runs `npm run proto` to regenerate `src/generated`.

## What Changes

- New page `src/app/(shop)/admin/users/page.tsx`, server-rendered, `dynamic = "force-dynamic"`, gated like `admin/audit/page.tsx`: no principal or token calls `redirectGuestToLogin("/admin/users", searchParams)` (the helper from `features/auth/guardPath`, so the Q1 guard test `src/app/guards.test.ts` stays green; no bare `redirect("/login")`); a principal without the `admin` scope gets the same `Result status="403"` ("Cần tài khoản Quản trị"); a gateway `PermissionDenied` renders the same 403; `Unauthenticated` calls `redirectGuestToLogin`.
- New gateway client `src/lib/gateway/users.ts` (`server-only`): `listUsers(filters, cursor)` and `setUserStatus(input)` through `makeClients(token)`; `makeClients` in `client.ts` gains an `accountAdmin` client for `AccountAdminService`. Results are discriminated unions (`ok`, `unauthenticated`, `forbidden`, `invalid`, `precondition`, `not_found`, `unavailable`) mapped from `ConnectError` codes; it never throws and never returns `err.message`. Every union member has a fixed text in the copy table below (`invalid` and `not_found` included).
- Search and filter live in the URL query string (`query`, `status`, `cursor`) through a GET form, so results are linkable and work without JS. `query` is a username prefix or an exact user id (the server decides) and is capped at 64 bytes in UTF-8 (identity rejects longer queries with `InvalidArgument`), cut at a character boundary. The server orders rows by username ascending; the page keeps that order. 20 rows per page (`pageSize: 20`), "Trang sau" link carries the response `next_cursor` and the filters.
- Table columns: Tên đăng nhập, ID, Vai trò, Trạng thái, Lý do, Hết hạn lúc, Thao tác. `status_until` is shown as `YYYY-MM-DD HH:mm` in UTC+07:00. `created_at` is not shown (no column).
- Row action form (server action `setUserStatusAction` in `src/features/admin/userActions.ts`): status select (Hoạt động, Hạn chế, Tạm khóa, Cấm), reason code select (required), note (optional, max 500 chars), and for "Tạm khóa" a required end time (`datetime-local`, read as UTC+07:00, must be in the future). The action re-checks the `admin` scope and the session on the server, validates locally (no gateway call on a local error), calls `SetUserStatus`, then revalidates the page. Success shows "Đã cập nhật trạng thái; đã đăng xuất N phiên." using `sessions_revoked` (RESTRICTED, SUSPENDED and BANNED, including re-setting the same status, revoke every session; ACTIVE revokes none, so a reinstatement shows N = 0).
- No action form on the signed-in admin's own row or on any row whose roles include `admin` (the server also refuses with `FAILED_PRECONDITION`; the message is shown if it happens).
- Login: `loginAction` maps `PermissionDenied` to a fixed message and stops showing the server `rawMessage` for it. The refresh path in `src/middleware.ts` treats `PermissionDenied` from `RefreshSession` like `Unauthenticated`: it clears both cookies, so a suspended or banned user is not retried on every request.
- The page sets `metadata.referrer = "no-referrer"` so the search query in the URL is not sent in a `Referer` header to any link target.
- Restricted seller (A20): every seller-center page guard sends only a visitor with no principal to login; a signed-in principal without `listing.write` (a restricted seller, or a buyer) is never redirected to `/login` or `/` and sees the seller layout notice, which gains the line "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục." under the existing "Cần tài khoản Người Bán" result. The token carries only scopes, so the frontend cannot tell a restricted seller from a buyer; one notice serves both.
- Cockpit page gains a link "Quản lý người dùng" to `/admin/users` (admin-gated like the audit link in `CockpitView`).

No gateway, identity or proto change here (the `AccountAdminService` gateway forwarders are owned by identity-user-enforcement). No new dependency. UI copy is proposed below for human review.

## Proposed Vietnamese copy (for the human to confirm)

| where | text |
|---|---|
| page title / h1 | "Quản lý người dùng" |
| metadata description | "Tìm người dùng và hạn chế, tạm khóa hoặc cấm tài khoản." |
| filter labels | "Tên đăng nhập hoặc ID", "Trạng thái" |
| buttons | "Tìm", "Xóa bộ lọc", "Trang sau", "Cập nhật trạng thái" |
| status labels | "Hoạt động", "Hạn chế", "Tạm khóa", "Cấm" |
| form labels | "Trạng thái mới", "Mã lý do", "Ghi chú (tối đa 500 ký tự)", "Tạm khóa đến" |
| reason codes (proposed, see open assumptions) | `policy_violation` "Vi phạm chính sách", `fraud` "Gian lận", `counterfeit` "Hàng giả", `abuse` "Lạm dụng", `scam` "Lừa đảo", `spam` "Spam", `other` "Khác" |
| empty state | "Không tìm thấy người dùng nào" |
| invalid input refused by the server (`invalid`) | "Dữ liệu không hợp lệ; trạng thái không thay đổi." |
| unknown target user (`not_found`) | "Không tìm thấy người dùng này; trạng thái không thay đổi." |
| missing reason | "Vui lòng chọn mã lý do." |
| missing or past end time | "Chọn thời điểm kết thúc tạm khóa trong tương lai." |
| note too long | "Ghi chú tối đa 500 ký tự." |
| success | "Đã cập nhật trạng thái; đã đăng xuất N phiên." |
| not allowed (self/admin target) | "Không thể thay đổi trạng thái của chính bạn hoặc của một quản trị viên khác." |
| audit down / service down | "Hệ thống chưa ghi nhận được thao tác; trạng thái không thay đổi. Vui lòng thử lại sau." |
| list unavailable | "Không tải được danh sách người dùng. Vui lòng thử lại sau." |
| 403 title / sub-text | "Cần tài khoản Quản trị" / "Tài khoản của bạn không có quyền admin để quản lý người dùng." |
| 403 from gateway | "Cần tài khoản Quản trị" / "Gateway từ chối quyền admin cho phiên đăng nhập này." |
| login refused (suspended or banned, one text, A16) | "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ." |
| seller layout, signed in without `listing.write` (A20) | existing "Cần tài khoản Người Bán" result plus "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục." |
| cockpit link | "Quản lý người dùng" |

## Gateway route check

Every gateway route this slice calls: `AccountAdminService.ListUsers` and `SetUserStatus` (new, registered in `team-gateway/internal/edge/server.go` and added to `adminScope` in `policy.go` by identity-user-enforcement, per the initiative hot-file list); `AuthService.Login` and `AuthService.RefreshSession` (exist today, `auth.go`). Nothing is missing; no gateway task is needed here. If identity-user-enforcement ships without the gateway registration the e2e track fails, and the gap is a slice-level defect to fix there.

## NFR

- Page render is one gateway call (`ListUsers`) after the session check; client timeout 5 s as in `audit.ts`; a timeout maps to `unavailable`, not a crash. The set-status action is one `SetUserStatus` call with a 10 s timeout (audit write + session revoke run in the server transaction).
- Page size fixed at 20; no looping over pages.
- Availability: team-identity or team-audit down degrades only this page (error banner; filters stay usable); no other page depends on it. The login and refresh paths add no call.
- Lockout visible within 1 minute end to end (initiative NFR): the frontend adds no cache; the next request after the gateway denylist update is refused and the middleware drops the cookies.
- No client JS is required for search and paging; the status form is a server action form.

## Data classification

- Shown to admins: username, user id, roles, status, `status_reason` (reason code), `status_until`. `created_at` is received but not shown. Purpose: enforcement against policy violations. Basis: platform duty to act against violations (Decree 85/2021), per the initiative; DPO sign-off is a NEEDS_HUMAN item of the initiative. Retention: the frontend stores nothing; identity keeps the record while the account exists.
- Entered by admins: reason code and a free-text note (<= 500 chars) that may contain personal data. It is sent only to `SetUserStatus`, is not logged by the frontend, is not placed in the URL, not sent to the tracker or analytics, and is not echoed back in the list (the list shows only the reason code).
- The search `query` (username prefix or id) travels in the URL query string; it is an opaque identifier, not contact data. The page uses GET deliberately, fires no analytics event and sets `Referrer-Policy` `no-referrer` (page metadata) so the query does not leak through the `Referer` header; the URL can still reach access logs, which is listed for the initiative's DPO sign-off.
- Login: the refusal message does not disclose whether the account is suspended or banned, why, or until when, and is shown only after the server accepted the password, so it does not enable username enumeration (server rule from the initiative).
- No new cookies, local storage or client persistence. The session bearer stays server-side (`server-only`).

## Threat model

- Assets: the ability to lock accounts (high impact), the admin session token, enforcement records.
- Abuse cases:
  1. A non-admin or guest opens `/admin/users` or posts the server action directly: a guest is redirected to login, a non-admin sees the 403 and the action refuses before any gateway call; the gateway (`adminScope`) and identity remain authoritative, and a `PermissionDenied` is shown as the 403 or an error.
  2. Stale scopes in the cookie: the gateway answers `PermissionDenied`; no rows are shown and no change is made.
  3. An admin locks themselves or another admin by forging the form: server refuses with `FAILED_PRECONDITION`; the UI hides the control and shows the refusal.
  4. Injection through username, reason or note rendered back: all values are rendered as React text, never HTML; inputs are passed as typed proto fields.
  5. Hostile query params: each param is trimmed and capped (query 64 bytes in UTF-8, cursor 256), an array value takes its first value, an unknown status value is ignored.
  6. Double submit or replay of the action: the UI disables the button while the action is pending; any repeat that does reach the server is audited like a first call (audit first, rule 6). CSRF is covered by the framework's server-action origin check, no extra token.
  7. Login message used to probe accounts: one fixed text, no `rawMessage` from the server for `PermissionDenied`.
- Authz rule: render and act only when `principal.scopes.includes("admin")`; identity and gateway decide.

## Open assumptions

- User-status reason codes: q6-trust-contract accepts any non-empty lower_snake_case code of at most 64 bytes and leaves the list to the admin UI. This slice offers `policy_violation | fraud | counterfeit | abuse | scam | spam | other` (`scam` matches identity's own tests) and sends the code as a plain string.
- The login refusal text does not tell suspended from banned and shows no end time (A16, decided).
- A restricted user loses `listing.write`; the seller-center pages show the notice above instead of a login redirect (A20). Listing write refusals on individual listings stay with frontend-listing-moderation.
- An expired suspension is lifted only at the user's next login (A15), so `/admin/users` can show a SUSPENDED row whose end time is past; the page shows it as returned.
- `ListUsers` returns users ordered by username ascending (identity-user-enforcement); the page keeps that order.
