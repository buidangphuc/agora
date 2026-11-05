## MODIFIED Requirements

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
