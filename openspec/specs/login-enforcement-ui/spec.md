# login-enforcement-ui Specification

## Purpose
The frontend tells suspended or banned users why login failed and shows restricted sellers a notice instead of a login loop.

## Requirements

### Requirement: A suspended or banned user is told so at login and is not retried

`loginAction` SHALL map `PermissionDenied` from `AuthService.Login` to the fixed text "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ." for both a suspended and a banned user, with no end time, and SHALL NOT show the server message for it. `Unauthenticated` and `InvalidArgument` SHALL keep the text "Tên đăng nhập hoặc mật khẩu không chính xác." and set no session cookie. When `AuthService.RefreshSession` answers `PermissionDenied`, `src/middleware.ts` SHALL clear both the access and refresh cookies, as it does for `Unauthenticated`; for other failures it SHALL keep the refresh cookie.

#### Scenario: Users UI: Suspended user sees the fixed refusal
- **WHEN** `Login` answers `PermissionDenied` with the raw message "user suspended until 2026-10-20"
- **THEN** the form error is exactly "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ.", contains no part of the raw message (no date), and no session cookie is set; a raw message "user banned" gives the same text
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `maps permission denied to the fixed refusal`

#### Scenario: Users UI: Wrong password keeps the credentials message
- **WHEN** `Login` answers `Unauthenticated`
- **THEN** the form error is "Tên đăng nhập hoặc mật khẩu không chính xác." and no session cookie is set
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `wrong password keeps the credentials message`

#### Scenario: Users UI: Other login failures are unchanged
- **WHEN** `Login` answers `Unavailable`
- **THEN** the form error starts with "Đăng nhập không thành công:" and no session cookie is set
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` `other failures keep the generic message`

#### Scenario: Users UI: Refresh of a suspended user clears both cookies
- **WHEN** a request carries an expired access cookie and a refresh cookie and `RefreshSession` answers `PermissionDenied`
- **THEN** the response deletes both `session` and the refresh cookie, and the page renders as a guest
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` `permission denied on refresh clears both cookies`

#### Scenario: Users UI: Transient refresh failure keeps the refresh cookie
- **WHEN** `RefreshSession` answers `Unavailable`
- **THEN** only the access cookie is deleted and the refresh cookie is kept for a retry
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` `transient refresh failure keeps the refresh cookie`

#### Scenario: Users UI: Suspended buyer cannot log in
- **WHEN** an admin has suspended a fresh buyer and that buyer submits the correct password on `/login`
- **THEN** the page shows the fixed refusal text and the browser stays on `/login`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Suspended buyer cannot log in`

#### Scenario: Users UI: Reinstated user can log in again
- **WHEN** an admin has set a banned buyer back to "Hoạt động" and the buyer logs in with the correct password
- **THEN** the browser leaves `/login` and the header shows the signed-in state
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Reinstated user can log in again`

#### Scenario: Users UI: Wrong password for a suspended buyer shows the credentials message
- **WHEN** a suspended buyer submits a wrong password on `/login`
- **THEN** the page shows "Tên đăng nhập hoặc mật khẩu không chính xác." and not the refusal text
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Wrong password does not reveal the suspension`

### Requirement: A restricted seller sees a notice on seller pages, not a login loop

Every page under `team-frontend-seller/src/app/seller/` SHALL call `redirectGuestToLogin` only when there is no session principal. A signed-in principal without the `listing.write` scope SHALL NOT be redirected; the seller layout SHALL render a result in place on the requested URL and SHALL NOT render the page content. When the principal's token carries `status: "restricted"` the result SHALL be a 403 titled "Tài khoản Người Bán đang bị hạn chế" showing the line "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục."; otherwise the result SHALL be the seller onboarding screen (capability `seller-onboarding-ui`) and SHALL NOT show "Cần tài khoản Người Bán" or the restricted line. When onboarding is refused with PERMISSION_DENIED the denied notice of `seller-onboarding-ui` SHALL replace it. The restricted notice SHALL NOT link to "Đăng ký bán hàng" and SHALL offer "Về trang mua sắm" and "Đăng xuất".

#### Scenario: Users UI: Seller layout shows the restricted notice
- **WHEN** the seller layout renders for a signed-in principal whose scopes lack `listing.write` and whose token carries `status: "restricted"`
- **THEN** it shows "Tài khoản Người Bán đang bị hạn chế" and the line "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục.", renders no children, shows no onboarding button, and `redirect` is called zero times
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `shows the restricted notice in place for a restricted seller`

#### Scenario: Users UI: Seller layout shows the seller-account notice to a buyer
- **WHEN** the seller layout renders for a signed-in principal whose scopes lack `listing.write` and whose token has no `status` claim
- **THEN** it shows the button "Đồng ý và trở thành Người Bán", does not show "Cần tài khoản Người Bán", the restricted line or a "Đăng ký bán hàng" link, renders no children, and `redirect` is called zero times
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `shows onboarding in place for a buyer`

#### Scenario: Users UI: Seller pages do not send a principal without the listing.write scope to login
- **WHEN** each seller page (`/seller`, `/seller/new`, `/seller/ads`, `/seller/plans`, `/seller/shop`, `/seller/wallet`, `/seller/bundles`, `/seller/analytics`, `/seller/orders`, `/seller/{id}/edit`) runs its guard for a signed-in principal without `listing.write`, and again with no principal
- **THEN** with the principal `redirect` is called zero times for every page; with no principal every page calls `redirectGuestToLogin` with its own path
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/restrictedGuard.test.tsx` `signed-in principal without listing.write is never redirected`

#### Scenario: Users UI: Restricted seller sees the notice in the browser
- **WHEN** an admin has restricted a freshly registered seller, and that seller logs in and opens `/seller/new` on the seller app
- **THEN** the browser stays on `/seller/new` and the page shows "Tài khoản Người Bán đang bị hạn chế" and "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục."
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Users UI: Restricted seller sees the notice in the browser`

#### Scenario: Users UI: Buyer sees the seller-account notice in the browser
- **WHEN** a freshly registered buyer logs in on the seller app and opens `/seller/new`
- **THEN** the browser stays on `/seller/new`, the page shows the button "Đồng ý và trở thành Người Bán", and it shows neither "Cần tài khoản Người Bán", the restricted line nor a "Đăng ký bán hàng" link
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/user_enforcement_ui.feature` `Users UI: Buyer sees the seller-account notice in the browser`
