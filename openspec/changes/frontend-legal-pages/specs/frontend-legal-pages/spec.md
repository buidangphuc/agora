## ADDED Requirements

### Requirement: Seven legal pages are public static pages in Vietnamese

team-frontend SHALL serve the pages `/dieu-khoan-su-dung`, `/chinh-sach-bao-mat`, `/quy-che-hoat-dong`, `/chinh-sach-doi-tra`, `/hang-cam-kinh-doanh`,
`/giai-quyet-khieu-nai` and `/dieu-khoan-nguoi-ban` to any caller including anonymous visitors, each rendered from `content/legal/<slug>.md` with a title
and a version header, and with the placeholder body "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP" until the legal owner supplies text. The pages SHALL be
statically rendered, SHALL make no gateway call, and SHALL be listed in the sitemap and not disallowed in `robots.txt`.

#### Scenario: A legal page renders title, version and placeholder
- **WHEN** an anonymous visitor opens `/dieu-khoan-su-dung`
- **THEN** the response is 200 with an `h1` in Vietnamese, the text "Phiên bản" followed by the front-matter version, and the text "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP"
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::A legal page renders title, version and placeholder

#### Scenario: All seven slugs resolve
- **WHEN** an anonymous visitor requests each of the seven slugs
- **THEN** each response is 200 and the sitemap contains exactly those 7 legal URLs
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::All seven slugs resolve

#### Scenario: An unknown legal slug is not found
- **WHEN** a visitor opens `/chinh-sach-khong-ton-tai`
- **THEN** the response is 404 and the not-found page renders
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::An unknown legal slug is not found

#### Scenario: Legal pages need no login
- **WHEN** a visitor without a session cookie opens `/chinh-sach-bao-mat`
- **THEN** the response is 200 with no redirect to `/login`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::Legal pages need no login

#### Scenario: Terms and privacy page versions equal the configured versions
- **WHEN** the unit suite reads `content/legal/dieu-khoan-su-dung.md` and `chinh-sach-bao-mat.md`
- **THEN** their `version` values equal `NEXT_PUBLIC_LEGAL_TERMS_VERSION` and `NEXT_PUBLIC_PRIVACY_POLICY_VERSION` and each is 1 to 32 characters, else the test fails
- **VERIFIED BY**: team-frontend/src/features/legal/content.test.ts "Terms and privacy page versions equal the configured versions"

### Requirement: The footer links the legal pages and carries the operator block

The consumer footer SHALL link all seven legal pages, SHALL link "Chính Sách Trả Hàng & Hoàn Tiền (RMA)" to `/chinh-sach-doi-tra` instead of
`/account/orders`, and SHALL show an operator block with four placeholder fields: company name, business registration number, address, and the Ministry
of Industry and Trade registration notice, each "[DO BỘ PHẬN PHÁP CHẾ CUNG CẤP]" until the legal owner fills them.

#### Scenario: Footer exposes the legal links
- **WHEN** any consumer page renders
- **THEN** the footer contains 7 links whose targets are the seven legal slugs and the RMA link target is `/chinh-sach-doi-tra`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::Footer exposes the legal links

#### Scenario: Footer shows the operator placeholders
- **WHEN** any consumer page renders
- **THEN** the footer contains the four labelled operator fields and each shows "[DO BỘ PHẬN PHÁP CHẾ CUNG CẤP]"
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::Footer shows the operator placeholders

#### Scenario: Footer never links legal pages that are missing
- **WHEN** the unit suite compares the footer link targets with the seven slugs
- **THEN** every footer legal link resolves to an existing page, no link points to `/account/orders` for the RMA entry, and an anonymous visitor sees the same footer without error
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_pages.feature::Footer never links legal pages that are missing

### Requirement: Sign-up requires accepting the current Terms and Privacy Policy

`RegisterForm` SHALL show two unchecked checkboxes, one for the Terms and one for the Privacy Policy, each linking to its page in a new tab, and
`registerAction` SHALL call `AuthService.Register` with `terms_version` and `privacy_version` taken from `NEXT_PUBLIC_LEGAL_TERMS_VERSION` and
`NEXT_PUBLIC_PRIVACY_POLICY_VERSION` only when both checkboxes are accepted. The action SHALL make no `Register` call when either acceptance is absent.

#### Scenario: Registering with both boxes checked records two consents
- **WHEN** a visitor submits a valid username and password with both checkboxes checked
- **THEN** the account is created, the visitor is redirected as before, and `ListMyConsents` for that user returns 2 records (TERMS, PRIVACY_POLICY) at the current versions with source `register`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_signup.feature::Registering with both boxes checked records two consents

#### Scenario: Unchecked box blocks submission
- **WHEN** a visitor submits the form with the Terms box unchecked
- **THEN** a Vietnamese field error is shown on that box, 0 requests reach the gateway and no account is created
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_signup.feature::Unchecked box blocks submission

#### Scenario: A forged post without acceptance fields is refused
- **WHEN** a client posts the register server action without the `acceptTerms` or `acceptPrivacy` field
- **THEN** the action returns `ok:false` with a field error and the gateway receives 0 `Register` calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_signup.feature::A forged post without acceptance fields is refused

#### Scenario: A stale version is reported to the user
- **WHEN** the build-time terms version differs from the identity current version and a visitor submits the form with both boxes checked
- **THEN** the identity response `INVALID_ARGUMENT` is shown as "Điều khoản đã được cập nhật. Vui lòng tải lại trang và đồng ý lại." and no account is created
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_signup.feature::A stale version is reported to the user

#### Scenario: Boundary version length
- **WHEN** the unit suite builds the register request with the configured versions
- **THEN** each version string is sent unchanged and is at most 32 characters
- **VERIFIED BY**: team-frontend/src/features/auth/actions.test.ts "Boundary version length"

### Requirement: Users with stale acceptance are asked to accept again

For a signed-in user the shell SHALL call `PrivacyService.ListMyConsents` at most once per tab session and, when the latest TERMS or latest PRIVACY_POLICY
record is absent or has a version different from the current configured version, SHALL show a blocking dialog "Đồng ý" / "Đăng xuất" on every route
except the legal pages. "Đồng ý" SHALL call `PrivacyService.RecordConsent` with TERMS and PRIVACY_POLICY at the current versions, `granted=true`,
source `account`. An anonymous visitor SHALL never see the dialog and SHALL cause 0 `PrivacyService` calls.

#### Scenario: A legacy user is prompted and accepts
- **WHEN** a signed-in user with no TERMS or PRIVACY_POLICY row loads `/`, then clicks "Đồng ý"
- **THEN** the dialog appeared before the click, `RecordConsent` was called once with 2 records, the dialog closes within 1 second, and a reload shows no dialog
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::A legacy user is prompted and accepts

#### Scenario: A newer Terms version triggers the prompt
- **WHEN** a signed-in user whose latest TERMS version is older than the current version loads any non-legal page
- **THEN** the dialog shows with 2 actions and the legal pages stay readable without it
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::A newer Terms version triggers the prompt

#### Scenario: Up-to-date users are not prompted
- **WHEN** a signed-in user whose latest TERMS and PRIVACY_POLICY versions equal the current ones loads 3 pages in one tab
- **THEN** no dialog shows and `ListMyConsents` is called at most once
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::Up-to-date users are not prompted

#### Scenario: Anonymous visitors are never prompted
- **WHEN** an anonymous visitor loads `/`
- **THEN** no dialog shows and the gateway receives 0 `PrivacyService` calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::Anonymous visitors are never prompted

#### Scenario: Consent lookup failure does not block browsing
- **WHEN** `ListMyConsents` fails with `UNAVAILABLE` for a signed-in user
- **THEN** no dialog shows, the page renders normally and no error is surfaced to the user
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::Consent lookup failure does not block browsing

#### Scenario: Declining signs the user out
- **WHEN** a prompted user clicks "Đăng xuất"
- **THEN** the session cookies are cleared, the user lands on `/` as anonymous and no consent row is written
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature::Declining signs the user out

### Requirement: The consent banner is Vietnamese and records signed-in choices

The banner SHALL use Vietnamese labels ("Tùy chọn cookie", "Phân tích", "Tiếp thị", "Cá nhân hóa", "Từ chối tất cả", "Lưu lựa chọn", "Chấp nhận tất cả",
"Cài đặt cookie") and SHALL keep the cookie behaviour of the `consent` capability unchanged. When a signed-in user commits a choice the frontend SHALL
also call `PrivacyService.RecordConsent` once with one record per purpose whose value changed (all three when no prior cookie exists), version `"1"`,
source `banner`. For an anonymous visitor it SHALL make no `PrivacyService` call.

#### Scenario: Banner text is Vietnamese
- **WHEN** the banner opens for a visitor with no cookie
- **THEN** the dialog title is "Tùy chọn cookie", the three groups are labelled "Phân tích", "Tiếp thị", "Cá nhân hóa", and no English label from the previous banner remains
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature::Banner text is Vietnamese

#### Scenario: Signed-in save records only changed purposes
- **WHEN** a signed-in user whose cookie is `a=1&m=0&p=0` turns on "Cá nhân hóa" and clicks "Lưu lựa chọn"
- **THEN** the cookie becomes `a=1&m=0&p=1`, `RecordConsent` is called once with exactly 1 record (PERSONALIZATION, granted, source `banner`) and `ListMyConsents` shows it as the newest row
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature::Signed-in save records only changed purposes

#### Scenario: First choice records all three purposes
- **WHEN** a signed-in user with no consent cookie clicks "Từ chối tất cả"
- **THEN** `RecordConsent` is called once with 3 records, all `granted=false`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature::First choice records all three purposes

#### Scenario: Anonymous choice is cookie only
- **WHEN** an anonymous visitor clicks "Chấp nhận tất cả"
- **THEN** the cookie is `v=1&a=1&m=1&p=1&ts=<unix>` and the gateway receives 0 `PrivacyService` calls
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature::Anonymous choice is cookie only

#### Scenario: Ledger write failure keeps the cookie choice
- **WHEN** a signed-in user commits a choice and `RecordConsent` fails with `UNAVAILABLE`
- **THEN** the cookie still holds the new choice, the banner closes and no error dialog is shown
- **VERIFIED BY**: platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature::Ledger write failure keeps the cookie choice

### Requirement: Seller onboarding links the seller terms and records acceptance

team-frontend-seller SHALL link the seller terms text to `/dieu-khoan-nguoi-ban` on the buyer app, and after `BecomeSeller` succeeds SHALL call
`PrivacyService.RecordConsent` with one SELLER_TERMS record (`granted=true`, version `SELLER_TERMS_VERSION`, source `seller_onboarding`).

#### Scenario: Terms link opens the seller terms page
- **WHEN** a user on `/seller/new` follows the terms link
- **THEN** the link target is the buyer app URL ending in `/dieu-khoan-nguoi-ban`, it opens in a new tab and the page returns 200
- **VERIFIED BY**: platform-e2e/tests/e2e/features/seller/seller_terms_consent.feature::Terms link opens the seller terms page

#### Scenario: Accepting seller terms records a ledger row
- **WHEN** a buyer clicks "Đồng ý và trở thành Người Bán" and `BecomeSeller` succeeds
- **THEN** `ListMyConsents` returns a SELLER_TERMS row with the seller terms version and source `seller_onboarding`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/seller/seller_terms_consent.feature::Accepting seller terms records a ledger row

#### Scenario: Denied BecomeSeller writes no consent row
- **WHEN** `BecomeSeller` fails with `PERMISSION_DENIED`
- **THEN** the denied screen shows and `RecordConsent` is called 0 times
- **VERIFIED BY**: platform-e2e/tests/e2e/features/seller/seller_terms_consent.feature::Denied BecomeSeller writes no consent row

#### Scenario: Consent write failure does not undo seller status
- **WHEN** `BecomeSeller` succeeds and `RecordConsent` fails with `UNAVAILABLE`
- **THEN** the user is redirected as a seller and no error is shown to the user
- **VERIFIED BY**: platform-e2e/tests/e2e/features/seller/seller_terms_consent.feature::Consent write failure does not undo seller status
