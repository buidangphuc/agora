# seller-onboarding-ui Specification

## Purpose
TBD - created by archiving change seller-onboarding-ui. Update Purpose after archive.

## Requirements

### Requirement: The seller app onboards a buyer instead of a dead end

For a signed-in principal without `listing.write` whose token has no `status: "restricted"`, the seller layout SHALL render in place on the requested URL an onboarding screen with the seller terms text, the current terms version and a button "Đồng ý và trở thành Người Bán", SHALL NOT render the page content, and SHALL NOT redirect. Accepting SHALL call `AuthService.BecomeSeller` with the displayed terms version, write the returned access token into the `seller_session` cookie without touching `seller_refresh`, and continue to `/seller/new`. `/sell` SHALL keep redirecting to `/seller/new`. The seller app SHALL NOT contain the text "Cần tài khoản Người Bán".

#### Scenario: Seller onboarding UI: a buyer sees the onboarding screen
- **WHEN** the seller layout renders for a signed-in principal whose scopes lack `listing.write` and whose token has no `status` claim
- **THEN** it shows the terms text, the terms version and the button "Đồng ý và trở thành Người Bán", renders no children, calls `redirect` zero times and shows no "Cần tài khoản Người Bán"
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `shows onboarding in place for a buyer`

#### Scenario: Seller onboarding UI: an existing seller never sees onboarding
- **WHEN** the seller layout renders for a principal with `listing.write`
- **THEN** it renders its children and the sidebar, and the button "Đồng ý và trở thành Người Bán" is absent
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `seller with listing.write sees no onboarding`

#### Scenario: Seller onboarding UI: accepting sends the displayed version and updates the session cookie
- **WHEN** the accept action runs with a `seller_session` cookie and `BecomeSeller` answers a new access token
- **THEN** `BecomeSeller` is called exactly once with the displayed terms version, `seller_session` holds the new token, `seller_refresh` is not written, and the action redirects to `/seller/new`
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `accept sets only the session cookie and redirects`

#### Scenario: Seller onboarding UI: the access token never reaches the client
- **WHEN** the accept action succeeds
- **THEN** the action result and the rendered HTML contain no part of the new access token
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `token is not exposed to the client`

#### Scenario: Seller onboarding UI: the accept action refuses to run without a session
- **WHEN** the accept action runs with no `seller_session` cookie
- **THEN** it returns an error, calls zero RPCs and sets no cookie
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `accept without session sets no cookie`

#### Scenario: Seller onboarding UI: an unknown terms version shows an error
- **WHEN** `BecomeSeller` answers INVALID_ARGUMENT
- **THEN** the onboarding screen stays with a visible error line, the button is enabled again and no cookie changes
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `invalid terms version shows error and keeps cookies`

#### Scenario: Seller onboarding UI: an unavailable gateway shows an error
- **WHEN** `BecomeSeller` answers UNAVAILABLE
- **THEN** the onboarding screen stays with a visible error line beginning "Không thể" and no cookie changes
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `unavailable shows error and keeps cookies`

#### Scenario: Seller onboarding UI: an expired session on accept goes to login
- **WHEN** `BecomeSeller` answers UNAUTHENTICATED
- **THEN** the action redirects to the seller login with `returnUrl` equal to `/seller/new` (the path the layout was rendered on) and sets no cookie
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `unauthenticated redirects to login`

#### Scenario: Seller onboarding UI: an empty token is an error
- **WHEN** `BecomeSeller` answers success with an empty access token
- **THEN** the onboarding screen stays with a visible error line, `seller_session` is unchanged and no redirect happens
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `empty token shows error and keeps cookies`

#### Scenario: Seller onboarding UI: the accept button is disabled while pending
- **WHEN** the buyer clicks the button and the action has not returned
- **THEN** the button is disabled, and a second click within that time sends no second action call
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerOnboarding.test.tsx` `button disabled while pending`

#### Scenario: Seller onboarding UI: the screens offer the shopping and logout actions
- **WHEN** the onboarding screen renders for a buyer, and when the denied notice renders
- **THEN** each of the two shows a link "Về trang mua sắm" to the buyer app and a button "Đăng xuất"
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerOnboarding.test.tsx` `onboarding and denied notice offer shopping and logout`

#### Scenario: Seller onboarding UI: /sell keeps redirecting
- **WHEN** a request opens `/sell`
- **THEN** the response redirects to `/seller/new`
- **VERIFIED BY**: `team-frontend-seller/src/app/sell/page.test.tsx` `sell redirects to seller new`

#### Scenario: Seller onboarding UI: a buyer becomes a seller in the browser
- **WHEN** a freshly registered buyer logs in on the seller app, opens `/sell` and clicks "Đồng ý và trở thành Người Bán"
- **THEN** the browser ends on `/seller/new` with the new-listing form visible, the `seller_session` cookie carries `listing.write`, the `seller_refresh` cookie value is unchanged, and the page shows no "Cần tài khoản Người Bán"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a buyer becomes a seller in the browser`

#### Scenario: Seller onboarding UI: no source file shows the removed text
- **WHEN** `Cần tài khoản Người Bán` is searched with grep across `team-frontend-seller` (excluding `node_modules`, `.next` and test files asserting its absence), `platform-e2e/tests/e2e/step_definitions` and the feature files
- **THEN** zero matches remain, except in assertions that the text is absent
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/removedText.test.ts` `no source shows the removed seller notice text` (static scan, cannot be produced end to end)

#### Scenario: Seller onboarding UI: a returning seller is not asked again
- **WHEN** the same user reloads `/seller` after becoming a seller
- **THEN** the seller centre renders and the onboarding screen is absent
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a returning seller is not asked again`

#### Scenario: Seller onboarding UI: an anonymous visitor is sent to login
- **WHEN** a visitor with no session opens `/sell`
- **THEN** the browser lands on the seller `/login` with `returnUrl` equal to the seller path, not on the onboarding screen
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: an anonymous visitor is sent to login`

### Requirement: A restricted, suspended or banned account gets no onboarding

A principal whose token carries `status: "restricted"` SHALL see the restricted notice and no onboarding button. Suspending or banning an account bulk-revokes its sessions, so a suspended or banned buyer's `BecomeSeller` answers UNAUTHENTICATED and the action SHALL redirect to the seller login with no onboarding and no cookie change. When `BecomeSeller` answers PERMISSION_DENIED, which is reachable only for an account restricted after the page loaded, the onboarding screen SHALL be replaced by a denied notice titled "Không thể trở thành Người Bán" with the line "Tài khoản của bạn không đủ điều kiện trở thành Người Bán. Vui lòng liên hệ bộ phận hỗ trợ.", no accept button, "Về trang mua sắm" and "Đăng xuất", and the `seller_session` cookie SHALL stay unchanged. The notice SHALL NOT include the raw server message.

#### Scenario: Seller onboarding UI: a denied acceptance shows the denied notice
- **WHEN** `BecomeSeller` answers PERMISSION_DENIED with the raw message "account restricted"
- **THEN** the screen shows the title "Không thể trở thành Người Bán" and the fixed line, no button "Đồng ý và trở thành Người Bán", no part of the raw message, and the `seller_session` cookie is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/onboarding.test.ts` `permission denied shows fixed denied notice`

#### Scenario: Seller onboarding UI: a restricted account sees no onboarding button
- **WHEN** the seller layout renders for a principal without `listing.write` whose token carries `status: "restricted"`
- **THEN** it shows "Tài khoản Người Bán đang bị hạn chế", renders no accept button and calls `BecomeSeller` zero times
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/layout.test.tsx` `restricted principal gets no onboarding`

#### Scenario: Seller onboarding UI: a suspended buyer is sent to login in the browser
- **WHEN** a buyer is signed in on the seller app, an admin sets the buyer to "suspended" (sessions revoked), and the buyer clicks "Đồng ý và trở thành Người Bán"
- **THEN** `BecomeSeller` answers UNAUTHENTICATED, the browser lands on the seller `/login`, no onboarding is shown, and the `seller_session` cookie does not carry `listing.write`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a suspended buyer is sent to login in the browser`

#### Scenario: Seller onboarding UI: a banned buyer is sent to login in the browser
- **WHEN** a buyer is signed in on the seller app, an admin sets the buyer to "banned" (sessions revoked), and the buyer clicks "Đồng ý và trở thành Người Bán"
- **THEN** `BecomeSeller` answers UNAUTHENTICATED, the browser lands on the seller `/login`, never reaches `/seller/new`, and the `seller_session` cookie does not carry `listing.write`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a banned buyer is sent to login in the browser`

#### Scenario: Seller onboarding UI: a restricted seller sees the inline 403 in the browser
- **WHEN** an admin has restricted a buyer, and that buyer logs in on the seller app and opens `/seller/new`
- **THEN** the browser stays on `/seller/new`, the page shows "Tài khoản Người Bán đang bị hạn chế", and no button "Đồng ý và trở thành Người Bán" is present
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a restricted seller sees the inline 403 in the browser`

#### Scenario: Seller onboarding UI: a buyer restricted after page load is refused
- **WHEN** a buyer sees onboarding, an admin sets the buyer to "restricted", and the buyer clicks the button
- **THEN** the page shows "Không thể trở thành Người Bán" and the `seller_session` cookie does not carry `listing.write`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_onboarding_ui.feature` `Seller onboarding UI: a buyer restricted after page load is refused`
