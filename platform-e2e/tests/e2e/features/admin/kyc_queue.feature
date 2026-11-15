@admin @kyc_queue
Feature: Admin KYC verification queue in the browser (admin-kyc-queue)
  /admin/cases?tab=kyc lists seller KYC submissions oldest first, opens documents only through the
  audited RPC and lets an admin approve or reject. Scenario names echo the spec scenarios of
  openspec/changes/admin-kyc-queue (capability admin-kyc-queue-ui). Submissions are created through
  the real RPCs (CreateKycUploadUrl, presigned PUT, SubmitKyc). Scenarios that need an exact queue
  first clear the PENDING queue through ReviewKyc. Every refusal is paired with a positive control.

  Scenario: KYC queue: Anonymous visitor is redirected to login
    Given seller "A" has a pending KYC submission
    And no admin session is open in the browser
    When the visitor opens the KYC tab
    Then the browser is sent to login with a return URL for the KYC tab
    And the KYC table is not shown

  Scenario: KYC queue: Non-admin sees the 403 result
    Given seller "A" has a pending KYC submission
    And seller "A" is signed in the browser
    When the visitor opens the KYC tab
    Then the 403 result "Cần tài khoản Quản trị" is shown
    And the KYC table is not shown
    When the seeded admin is signed in the browser
    And the visitor opens the KYC tab
    Then the submission of seller "A" is listed

  Scenario: KYC queue: Tab appears next to the existing tabs
    Given the seeded admin is signed in the browser
    When the visitor opens "/admin/cases?tab=reports"
    Then the tab strip shows "Báo cáo", "Hàng chờ duyệt" and "Xác minh người bán"
    And the tab "Xác minh người bán" links to "/admin/cases?tab=kyc"

  @destructive
  Scenario: KYC queue: Pending submissions are listed oldest first
    Given the PENDING KYC queue is empty
    And sellers "A", "B" and "C" each submit KYC one after another
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    Then the table shows 3 rows in the order of sellers "A", "B", "C"
    And each row shows the seller user id, its document count and its creation time in UTC+07:00

  @destructive
  Scenario: KYC queue: Page boundary at 20 rows
    Given the PENDING KYC queue is empty
    And 21 sellers each have a pending KYC submission
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    Then the table shows 20 rows and a "Trang sau" link
    When the visitor follows "Trang sau"
    Then the table shows 1 rows and no "Trang sau" link

  @destructive
  Scenario: KYC queue: Empty queue
    Given the PENDING KYC queue is empty
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    Then the empty message "Không có hồ sơ nào đang chờ duyệt." is shown
    And the table shows 0 rows

  Scenario: KYC queue: Decided submissions are reachable through the filter
    Given seller "A" has a pending KYC submission
    And the seeded admin rejects the submission of seller "A" with reason code "document_unreadable"
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab with status "REJECTED"
    Then the row of seller "A" shows "Đã từ chối" and "document_unreadable"
    And the row of seller "A" has no "Duyệt" or "Từ chối" button
    When the visitor opens the KYC tab
    Then the submission of seller "A" is not listed

  Scenario: KYC queue: Viewing a document reveals a 5-minute link and writes the audit entry
    Given seller "A" has a pending KYC submission with 2 documents
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Xem tài liệu 1" on the row of seller "A"
    Then the row of seller "A" shows a "Mở tài liệu" link that opens in a new tab with rel "noopener noreferrer"
    And the link expires at most 5 minutes after the click
    And team-audit holds 1 kyc.document_view events for the submission of seller "A" by the seeded admin

  Scenario: KYC queue: Each view is a separate audited call
    Given seller "A" has a pending KYC submission with 2 documents
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Xem tài liệu 1" on the row of seller "A"
    And the admin clicks "Xem tài liệu 1" on the row of seller "A"
    And the admin clicks "Xem tài liệu 2" on the row of seller "A"
    Then team-audit holds 3 kyc.document_view events for the submission of seller "A" by the seeded admin

  Scenario: KYC queue: No document URL is present before the click
    Given seller "A" has a pending KYC submission with 2 documents
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    Then the submission of seller "A" is listed
    And the rendered page contains no presigned document URL
    And team-audit holds 0 kyc.document_view events for the submission of seller "A" by the seeded admin

  @destructive
  Scenario: KYC queue: Audit store down fails closed
    Given seller "A" has a pending KYC submission with 2 documents
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And team-audit is down for the KYC queue
    And the admin clicks "Xem tài liệu 1" on the row of seller "A"
    Then the row of seller "A" shows the alert "Không ghi được nhật ký truy cập nên chưa mở được tài liệu. Vui lòng thử lại sau."
    And the row of seller "A" has no "Mở tài liệu" link
    And the rendered page contains no presigned document URL

  Scenario: KYC queue: Document button count matches the document count
    Given seller "A" has a pending KYC submission with 3 documents
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    Then the row of seller "A" has exactly 3 document buttons named "Xem tài liệu 1" to "Xem tài liệu 3"
    And the row of seller "A" has no button "Xem tài liệu 4"

  Scenario: KYC queue: Approve removes the row and verifies the seller
    Given seller "A" has a pending KYC submission
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Duyệt" on the row of seller "A"
    Then the toast "Đã duyệt hồ sơ xác minh." is shown
    And the submission of seller "A" is no longer listed as pending
    And the verification status of seller "A" is "VERIFICATION_STATUS_VERIFIED"

  Scenario: KYC queue: Reject with a reason code
    Given seller "A" has a pending KYC submission
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Từ chối" on the row of seller "A"
    And the admin picks the reason "Giấy tờ không đọc được" and confirms
    Then the toast "Đã từ chối hồ sơ xác minh." is shown
    And the submission of seller "A" is no longer listed as pending
    And the gateway lists the submission of seller "A" as rejected with reason code "document_unreadable"
    When the visitor opens the KYC tab with status "REJECTED"
    Then the row of seller "A" shows "Đã từ chối" and "document_unreadable"

  Scenario: KYC queue: Reject without a reason is blocked
    Given seller "A" has a pending KYC submission
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Từ chối" on the row of seller "A"
    And the admin confirms the reject dialog without a reason
    Then the dialog shows "Vui lòng chọn lý do."
    And the gateway lists the submission of seller "A" as pending

  Scenario: KYC queue: Reviewing your own submission is refused
    Given a second admin who has a pending KYC submission is signed in the browser
    When the visitor opens the KYC tab
    And the admin clicks "Duyệt" on the row of their own submission
    Then the toast "Bạn không thể xử lý hồ sơ của chính mình hoặc không có quyền thực hiện thao tác này." is shown
    And the gateway lists the submission of the admin as pending

  Scenario: KYC queue: Second decision on the same submission fails
    Given seller "A" has a pending KYC submission
    And a second admin
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And the second admin approves the submission of seller "A" through the gateway
    And the admin clicks "Duyệt" on the row of seller "A"
    Then the toast "Hồ sơ đã được xử lý hoặc không còn ở trạng thái chờ." is shown
    And the submission of seller "A" is no longer listed as pending
    And the gateway lists the submission of seller "A" as verified

  @destructive
  Scenario: KYC queue: Review failure leaves the row unchanged
    Given seller "A" has a pending KYC submission
    And the seeded admin is signed in the browser
    When the visitor opens the KYC tab
    And team-audit is down for the KYC queue
    And the admin clicks "Duyệt" on the row of seller "A"
    Then the toast "Không xử lý được hồ sơ. Vui lòng thử lại sau." is shown
    When team-audit is up again for the KYC queue
    Then the gateway lists the submission of seller "A" as pending
    And the row of seller "A" still shows the "Duyệt" button after reload
