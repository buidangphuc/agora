# admin-kyc-queue-ui Specification

## Purpose
TBD - created by archiving change admin-kyc-queue. Update Purpose after archive.

## Requirements

### Requirement: The KYC tab is admin-only

`/admin/cases?tab=kyc` SHALL redirect a request with no session principal or token to login through `redirectGuestToLogin`,
SHALL render the admin layout's `Result status="403"` titled "Không có quyền" when the principal lacks the `admin` scope, and SHALL
NOT call the gateway in either case. A gateway `PermissionDenied` SHALL render a 403 result in the page and `Unauthenticated` SHALL redirect to login.
The tab strip on every `/admin/cases` tab SHALL include "Xác minh người bán" linking to `/admin/cases?tab=kyc`.

#### Scenario: KYC queue: Anonymous visitor is redirected to login
- **WHEN** a request with no session cookie opens `/admin/cases?tab=kyc`
- **THEN** the page redirects to `/login` with a return URL for the same page and `ListKycSubmissions` is called 0 times
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Anonymous visitor is redirected to login

#### Scenario: KYC queue: Non-admin sees the 403 result
- **WHEN** a signed-in buyer or seller without the `admin` scope opens `/admin/cases?tab=kyc`
- **THEN** the admin layout renders status 403 titled "Không có quyền" and `ListKycSubmissions` is called 0 times
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Non-admin sees the 403 result

#### Scenario: KYC queue: Gateway refusal is shown as the same 403
- **WHEN** the principal carries the `admin` scope but the gateway answers `PermissionDenied` to `ListKycSubmissions`
- **THEN** the page renders the 403 result with the sub-text "Gateway từ chối quyền admin cho phiên đăng nhập này." and no table
- **VERIFIED BY**: team-frontend-admin/src/app/admin/cases/page.test.tsx `kyc tab: gateway PermissionDenied renders the 403 result (KYC queue: Gateway refusal is shown as the same 403)`

#### Scenario: KYC queue: Expired session is sent to login
- **WHEN** the gateway answers `Unauthenticated` to `ListKycSubmissions` for an admin principal
- **THEN** the page redirects to `/login`
- **VERIFIED BY**: team-frontend-admin/src/app/admin/cases/page.test.tsx `kyc tab: Unauthenticated redirects to login (KYC queue: Expired session is sent to login)`

#### Scenario: KYC queue: Tab appears next to the existing tabs
- **WHEN** an admin opens `/admin/cases?tab=reports`
- **THEN** the tab strip shows "Báo cáo", "Hàng chờ duyệt" and "Xác minh người bán" and the last links to `/admin/cases?tab=kyc`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Tab appears next to the existing tabs

### Requirement: The queue lists submissions oldest first with a status filter and paging

The KYC tab SHALL call `ListKycSubmissions` with the status from the `status` query parameter (`PENDING` default; `VERIFIED` and
`REJECTED` accepted case-insensitively; any other value falls back to `PENDING`), page size 20 and the `cursor` parameter
(trimmed, at most 256 characters), and SHALL render the rows in the order returned (oldest first). Each row SHALL show the
submission id, user id, document type, document count, submitted time formatted `YYYY-MM-DD HH:mm:ss` in UTC+07:00, status
text and reason code. A "Trang sau" link carrying the next cursor SHALL appear only when the response has a next cursor.

#### Scenario: KYC queue: Pending submissions are listed oldest first
- **WHEN** an admin opens `/admin/cases?tab=kyc` while 3 submissions are `PENDING`, created at t1 < t2 < t3
- **THEN** the table shows 3 rows in the order t1, t2, t3, each with its user id, document count and time in UTC+07:00
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Pending submissions are listed oldest first

#### Scenario: KYC queue: Page boundary at 20 rows
- **WHEN** 21 submissions are `PENDING` and the admin opens the tab
- **THEN** the table shows 20 rows and a "Trang sau" link, and following it shows the 21st row and no "Trang sau" link
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Page boundary at 20 rows

#### Scenario: KYC queue: Unknown status or oversized cursor falls back
- **WHEN** an admin opens `/admin/cases?tab=kyc&status=bogus&cursor=` followed by 300 characters
- **THEN** the page does not error, calls `ListKycSubmissions` with status `PENDING` and a cursor of exactly 256 characters
- **VERIFIED BY**: team-frontend-admin/src/features/admin/casesParams.test.ts `kyc status falls back to PENDING and cursor is capped at 256 (KYC queue: Unknown status or oversized cursor falls back)`

#### Scenario: KYC queue: Empty queue
- **WHEN** no submission is `PENDING` and an admin opens the tab
- **THEN** the page shows the empty message "Không có hồ sơ nào đang chờ duyệt." and no table rows
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Empty queue

#### Scenario: KYC queue: Decided submissions are reachable through the filter
- **WHEN** an admin opens `/admin/cases?tab=kyc&status=REJECTED` while one submission was rejected with reason code `document_unreadable`
- **THEN** the row shows status "Đã từ chối", the reason code and no "Duyệt" or "Từ chối" buttons
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Decided submissions are reachable through the filter

#### Scenario: KYC queue: List failure shows a retry message
- **WHEN** `ListKycSubmissions` fails with `UNAVAILABLE` or times out after 5 seconds
- **THEN** the tab strip stays visible and the page shows the alert "Không tải được danh sách hồ sơ xác minh. Vui lòng thử lại sau." with no upstream address or stack text
- **VERIFIED BY**: team-frontend-admin/src/app/admin/cases/page.test.tsx `kyc tab: list failure shows the retry alert (KYC queue: List failure shows a retry message)`

### Requirement: Documents are opened only through the audited RPC

Clicking "Xem tài liệu N" on a row SHALL call `GetKycDocumentUrl` once with that `submission_id` and `index = N-1`
(0 to `document_count - 1`), and only after a successful response SHALL reveal a link "Mở tài liệu" with `target="_blank"` and
`rel="noopener noreferrer"` plus the expiry time. The page SHALL NOT contain any document URL before that click, SHALL NOT log
or store the URL, and SHALL NOT retry the call automatically. Buttons SHALL exist only for indices below `document_count`.

#### Scenario: KYC queue: Viewing a document reveals a 5-minute link and writes the audit entry
- **WHEN** an admin clicks "Xem tài liệu 1" on a pending submission with 2 documents
- **THEN** one `kyc.document_view` audit entry exists with the admin as actor and that submission id, and the row shows a "Mở tài liệu" link whose expiry is at most 5 minutes after the click
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Viewing a document reveals a 5-minute link and writes the audit entry

#### Scenario: KYC queue: Each view is a separate audited call
- **WHEN** an admin clicks "Xem tài liệu 1" twice and "Xem tài liệu 2" once for the same submission
- **THEN** the audit log holds 3 `kyc.document_view` entries for that submission
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Each view is a separate audited call

#### Scenario: KYC queue: No document URL is present before the click
- **WHEN** an admin opens the tab and no "Xem tài liệu" button was clicked
- **THEN** the rendered page contains 0 presigned URLs and `GetKycDocumentUrl` is called 0 times
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: No document URL is present before the click

#### Scenario: KYC queue: Audit store down fails closed
- **WHEN** an admin clicks "Xem tài liệu 1" while the gateway answers `UNAVAILABLE` because the audit entry cannot be written
- **THEN** the row shows the alert "Không ghi được nhật ký truy cập nên chưa mở được tài liệu. Vui lòng thử lại sau." and no "Mở tài liệu" link exists
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Audit store down fails closed

#### Scenario: KYC queue: Document button count matches the document count
- **WHEN** a submission has `document_count` 3
- **THEN** the row shows exactly 3 buttons "Xem tài liệu 1" to "Xem tài liệu 3" and no button for index 3
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Document button count matches the document count

#### Scenario: KYC queue: A non-admin cannot obtain a document link
- **WHEN** a session without the `admin` scope invokes the view-document server action
- **THEN** the action returns the message "Bạn không có quyền thực hiện thao tác này." and `GetKycDocumentUrl` is called 0 times
- **VERIFIED BY**: team-frontend-admin/src/features/admin/kycActions.test.ts `KYC queue: A non-admin cannot obtain a document link - viewKycDocumentAction refuses a non-admin`

#### Scenario: KYC queue: Unknown submission or index is refused
- **WHEN** the gateway answers `NOT_FOUND` or `INVALID_ARGUMENT` to `GetKycDocumentUrl`
- **THEN** the row shows "Không tìm thấy tài liệu này." and no link
- **VERIFIED BY**: team-frontend-admin/src/features/admin/kycActions.test.ts `KYC queue: Unknown submission or index is refused - viewKycDocumentAction maps not_found and invalid to the not found message (%s)`

### Requirement: Admins approve or reject pending submissions

"Duyệt" and "Từ chối" SHALL appear only on `PENDING` rows. Approve SHALL call `ReviewKyc(id, "approve")` with an empty
`reason_code`. Reject SHALL open a dialog requiring one reason code from the fixed list `document_unreadable`,
`document_expired`, `document_mismatch`, `document_type_invalid`, `suspected_fraud`, `other` and SHALL call
`ReviewKyc(id, "reject", reason_code)`; confirming without a reason SHALL NOT call the RPC. The server action SHALL validate
the decision and reason against the fixed values before calling the gateway. After success the page SHALL refresh and the
row SHALL leave the `PENDING` list.

#### Scenario: KYC queue: Approve removes the row and verifies the seller
- **WHEN** an admin clicks "Duyệt" on a pending submission
- **THEN** a success toast "Đã duyệt hồ sơ xác minh." shows, the row is gone from the pending list, and `GetVerificationStatus` for that seller returns `VERIFIED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Approve removes the row and verifies the seller

#### Scenario: KYC queue: Reject with a reason code
- **WHEN** an admin clicks "Từ chối", picks "Giấy tờ không đọc được" and confirms
- **THEN** `ReviewKyc` is called with decision `reject` and `reason_code` `document_unreadable`, a toast "Đã từ chối hồ sơ xác minh." shows, and the `REJECTED` filter lists the row with that reason code
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Reject with a reason code

#### Scenario: KYC queue: Reject without a reason is blocked
- **WHEN** an admin confirms the reject dialog without choosing a reason
- **THEN** the dialog shows "Vui lòng chọn lý do." and `ReviewKyc` is called 0 times
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Reject without a reason is blocked

#### Scenario: KYC queue: Forged decision or reason is refused by the action
- **WHEN** the review server action is invoked with decision `ban` or with reason code `Bad Reason!`
- **THEN** the action returns "Dữ liệu không hợp lệ; hồ sơ không thay đổi." and `ReviewKyc` is called 0 times
- **VERIFIED BY**: team-frontend-admin/src/features/admin/kycActions.test.ts `KYC queue: Forged decision or reason is refused by the action - reviewKycAction refuses a forged decision or reason code (%s)`

#### Scenario: KYC queue: Reviewing your own submission is refused
- **WHEN** an admin who is also the submitting user clicks "Duyệt" on their own submission and the service answers `PERMISSION_DENIED`
- **THEN** an error toast "Bạn không thể xử lý hồ sơ của chính mình hoặc không có quyền thực hiện thao tác này." shows and the row stays `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Reviewing your own submission is refused

#### Scenario: KYC queue: A non-admin cannot review
- **WHEN** a session without the `admin` scope invokes the review server action
- **THEN** the action returns "Bạn không có quyền thực hiện thao tác này." and `ReviewKyc` is called 0 times
- **VERIFIED BY**: team-frontend-admin/src/features/admin/kycActions.test.ts `KYC queue: A non-admin cannot review - reviewKycAction refuses a non-admin`

#### Scenario: KYC queue: Second decision on the same submission fails
- **WHEN** two admins both click "Duyệt" on the same submission and the second `ReviewKyc` answers `FAILED_PRECONDITION`
- **THEN** the second admin sees "Hồ sơ đã được xử lý hoặc không còn ở trạng thái chờ." and the row disappears from their pending list after refresh
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Second decision on the same submission fails

#### Scenario: KYC queue: Review failure leaves the row unchanged
- **WHEN** `ReviewKyc` times out or answers `UNAVAILABLE` (for example the audit write failed)
- **THEN** an error toast "Không xử lý được hồ sơ. Vui lòng thử lại sau." shows and the row stays `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/admin/kyc_queue.feature: KYC queue: Review failure leaves the row unchanged
