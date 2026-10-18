# listing-report-ui Specification

## Purpose
A signed-in buyer can report a listing from its product page.

## Requirements

### Requirement: A signed-in buyer can report a listing from its page

The product page SHALL show a "Báo cáo tin đăng" control to a signed-in principal who is not the listing's seller. The control SHALL open a dialog with a required reason code (`prohibited_item`, `counterfeit`, `scam`, `misleading`, `inappropriate`, `other`) and an optional note of at most 500 characters, and SHALL call `CreateReport` once with target type `LISTING` and the listing id. The action SHALL map `FailedPrecondition` (own listing) to "Bạn không thể báo cáo tin đăng của chính mình.", `PermissionDenied` (for example a restricted user without the write scope) to "Bạn không có quyền thực hiện thao tác này.", and `ResourceExhausted` (20 open reports per reporter) to "Bạn đang có quá nhiều báo cáo chờ xử lý. Vui lòng thử lại sau khi các báo cáo được xử lý.", keeping the dialog open and the typed note, and SHALL never show `err.message`. A guest SHALL see a link to login whose return URL is the product page. The seller of the listing SHALL NOT see the control. The note SHALL NOT be logged, stored client-side or sent to the tracker.

#### Scenario: Moderation UI: Signed-in buyer sees the report control
- **WHEN** a signed-in buyer who is not the seller opens a product page
- **THEN** the page contains the "Báo cáo tin đăng" button
- **VERIFIED BY**: `team-frontend/src/features/listing/ReportListingButton.test.tsx` `renders for a signed-in non-owner`

#### Scenario: Moderation UI: Seller does not see the control on their own listing
- **WHEN** the seller of a listing opens its product page
- **THEN** the page contains no "Báo cáo tin đăng" control
- **VERIFIED BY**: `team-frontend/src/app/(shop)/listing/[id]/page.test.tsx` `hides the report control from the owner`

#### Scenario: Moderation UI: Guest is sent to login and back
- **WHEN** a guest clicks "Báo cáo tin đăng" on a product page
- **THEN** the browser goes to `/login` with the return URL `/listing/<id>`, and after login lands on the product page
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/listing_report_ui.feature` `Moderation UI: Guest is sent to login and back`

#### Scenario: Moderation UI: Submit a report with reason and note
- **WHEN** a buyer picks reason `scam`, types the note "Giá quá rẻ, yêu cầu chuyển khoản ngoài" and submits
- **THEN** one `CreateReport` call with target type LISTING, the listing id, reason `scam` and that note is sent, the dialog closes and the toast "Đã gửi báo cáo. Cảm ơn bạn." appears
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `sends one CreateReport call`

#### Scenario: Moderation UI: Reason is required
- **WHEN** a buyer submits with no reason selected
- **THEN** the submit is blocked, no call is made and the dialog shows "Vui lòng chọn lý do."
- **VERIFIED BY**: `team-frontend/src/features/listing/ReportListingButton.test.tsx` `blocks submit without a reason`

#### Scenario: Moderation UI: Report note length boundary
- **WHEN** a buyer types 500 characters, and then a 501st
- **THEN** the counter shows 500/500, the input accepts no 501st character and the action rejects a 501-character note locally with no gateway call
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `validates note length up to 500`

#### Scenario: Moderation UI: Report double submit is blocked
- **WHEN** a buyer double-clicks "Gửi báo cáo" while the first call is pending
- **THEN** exactly one `CreateReport` call is made
- **VERIFIED BY**: `team-frontend/src/features/listing/ReportListingButton.test.tsx` `pending state blocks a double submit`

#### Scenario: Moderation UI: Reporting twice is quietly accepted
- **WHEN** the same buyer reports the same listing again while the first report is still open and the gateway returns the existing report
- **THEN** the dialog closes with the same success toast and shows no error
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `an existing open report is a success`

#### Scenario: Moderation UI: Seller reporting their own listing is refused
- **WHEN** the gateway answers `FailedPrecondition` to a report on the caller's own listing
- **THEN** the dialog shows "Bạn không thể báo cáo tin đăng của chính mình." and stays open
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `maps own-listing refusal`

#### Scenario: Moderation UI: Reporter without permission is refused
- **WHEN** the gateway answers `PermissionDenied` to a report (a caller without the write scope, such as a restricted user)
- **THEN** the dialog shows "Bạn không có quyền thực hiện thao tác này.", stays open, keeps the typed note and does not show the own-listing message
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `maps permission denied apart from own listing`

#### Scenario: Moderation UI: Open report cap is explained
- **WHEN** the gateway answers `ResourceExhausted` because the buyer already has 20 open reports
- **THEN** the dialog shows "Bạn đang có quá nhiều báo cáo chờ xử lý. Vui lòng thử lại sau khi các báo cáo được xử lý.", stays open and keeps the typed note
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `maps resource exhausted to the cap message`

#### Scenario: Moderation UI: Expired session while reporting
- **WHEN** the gateway answers `Unauthenticated`
- **THEN** the dialog shows "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." and keeps the typed note
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `maps unauthenticated`

#### Scenario: Moderation UI: Report backend unavailable
- **WHEN** the gateway answers `Unavailable` or any unknown error
- **THEN** the dialog shows "Chưa gửi được báo cáo. Vui lòng thử lại sau.", keeps the typed note, and never shows `err.message`
- **VERIFIED BY**: `team-frontend/src/features/listing/reportActions.test.ts` `maps unavailable and unknown errors`

#### Scenario: Moderation UI: A buyer's report reaches the admin inbox
- **WHEN** a buyer reports a published listing with reason `scam` and a note in the browser, and an admin opens `/admin/cases`
- **THEN** the report is listed under "Đang mở" for that listing with the buyer as reporter
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/listing_report_ui.feature` `Moderation UI: A buyer's report reaches the admin inbox`

#### Scenario: Moderation UI: The seller never sees the reporter
- **WHEN** a buyer has reported a listing and its seller opens the seller center and the listing edit page
- **THEN** neither page contains the reporter's id or username or the report note
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/listing_report_ui.feature` `Moderation UI: The seller never sees the reporter`
