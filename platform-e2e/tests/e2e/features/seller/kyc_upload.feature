@seller @seller_kyc_ui
Feature: Seller verification page and ID document upload
  The seller app serves /seller/verification: the signed-in seller's own KYC status in Vietnamese and,
  when no submission is pending or verified, an upload form that sends each file to a presigned URL
  and then submits the documents. Statuses are reached only through the real KYC RPCs (SubmitKyc and
  an admin ReviewKyc); faults are real service stops, never intercepted routes.
  (seller-kyc-ui)

  # ── Status ─────────────────────────────────────────────────────────────
  @needsSeller
  Scenario: A seller who never submitted sees NOT_SUBMITTED and the form
    Given a seeded seller is logged in
    And the seller has no KYC submission
    And another seller was approved by an admin through ReviewKyc
    When the seller opens the verification page
    Then the verification page shows "Chưa gửi hồ sơ" and the upload form
    And the verification page shows only the signed-in seller's own status

  @needsSeller
  Scenario: A pending submission hides the form
    Given a seeded seller is logged in
    And the seller has a PENDING KYC submission
    When the seller opens the verification page
    Then the verification page shows "Đang chờ duyệt" and renders 0 file inputs

  @needsSeller
  Scenario: A verified seller sees the verified state
    Given a seeded seller is logged in
    And the seller was approved by an admin through ReviewKyc
    When the seller opens the verification page
    Then the verification page shows "Đã xác minh" and renders 0 file inputs

  @needsSeller
  Scenario: A rejected submission shows the reason and allows resubmission
    Given a seeded seller is logged in
    And the seller was rejected by an admin through ReviewKyc with reason code "document_unreadable"
    When the seller opens the verification page
    Then the verification page shows "Bị từ chối" with the text for that reason and the upload form again
    And the raw reason code "document_unreadable" is not rendered

  @needsSeller @destructive
  Scenario: Status read failure is shown as an error, not as not-submitted
    Given a seeded seller is logged in
    And team-verification is stopped for the seller UI
    When the seller opens the verification page
    Then the verification page shows an error alert with a "Thử lại" link and renders 0 upload forms
    And the verification page does not show "Chưa gửi hồ sơ"

  Scenario: A guest is sent to login
    When an anonymous visitor opens the verification page
    Then the browser is sent to the login page with the return path "/seller/verification"

  @needsBuyer
  Scenario: A buyer without seller scope cannot use the page
    Given a signed-in buyer without seller scope is on the seller app
    When the seller opens the verification page
    Then the seller access-denied view is shown and the verification page content is not

  # ── Upload ─────────────────────────────────────────────────────────────
  @needsSeller
  Scenario: Three valid files are uploaded and submitted
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects 3 valid files, ticks the consent checkbox and presses "Gửi hồ sơ xác minh"
    Then the browser sent 3 uploads with the matching Content-Type and Content-Length
    And the seller's KYC submission holds 3 documents
    And the verification page shows "Đang chờ duyệt" and renders 0 file inputs

  @needsSeller
  Scenario: Consent unticked blocks submission
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects 1 valid file and leaves consent unticked
    Then the submit button is disabled
    And the browser made 0 upload requests and the seller has no KYC submission

  @needsSeller
  Scenario: A file over 5 MiB is rejected before upload
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects a PNG of 5242881 bytes
    Then the form shows "Tệp tối đa 5 MiB" and the submit button is disabled
    And the browser made 0 upload requests

  @needsSeller
  Scenario: A file of exactly 5 MiB is accepted
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects a PNG of 5242880 bytes
    And the seller ticks the consent checkbox
    Then the form shows no size error and the submit button is enabled

  @needsSeller
  Scenario Outline: A wrong file type or an empty file is rejected
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects a <kind> file named "<name>" of <size> bytes
    And the seller ticks the consent checkbox
    Then the form shows "<message>" and the submit button is disabled

    Examples:
      | kind | name      | size | message                    |
      | GIF  | scan.gif  | 2048 | Chỉ nhận JPG, PNG hoặc PDF |
      | PNG  | empty.png | 0    | Tệp rỗng                   |

  @needsSeller
  Scenario: More than three files are refused
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects 4 valid files
    And the seller ticks the consent checkbox
    Then the form shows "Tối đa 3 tệp" and the submit button is disabled
    And the browser made 0 upload requests

  @needsSeller @destructive
  Scenario: A failed PUT stops the submission
    Given a seeded seller is logged in
    And the seller has no KYC submission
    And the KYC document storage is stopped
    When the seller opens the verification page
    And the seller selects 2 valid files, ticks the consent checkbox and presses "Gửi hồ sơ xác minh"
    Then the form shows "Tải tệp thất bại, vui lòng thử lại"
    And the seller has no KYC submission
    When the KYC document storage is started again
    And the seller presses "Gửi hồ sơ xác minh" again without re-selecting files
    Then the verification page shows "Đang chờ duyệt" and renders 0 file inputs

  @needsSeller
  Scenario: Submit cannot be sent twice
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects 1 valid file and ticks the consent checkbox
    And the seller double-clicks "Gửi hồ sơ xác minh"
    Then the verification page shows "Đang chờ duyệt" and renders 0 file inputs
    And the seller has exactly 1 KYC submission

  @needsSeller
  Scenario: Image bytes and keys never reach logs or the server action payload
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the verification page
    And the seller selects 1 valid file, ticks the consent checkbox and presses "Gửi hồ sơ xác minh"
    Then the verification page shows "Đang chờ duyệt" and renders 0 file inputs
    And no request to the seller app carried the file bytes
    And the page content contains no presigned URL and no document reference

  # ── Navigation ─────────────────────────────────────────────────────────
  @needsSeller
  Scenario: Nav lists the verification page
    Given a seeded seller is logged in
    When the seller visits "/seller"
    Then the navigation contains exactly 1 entry "Xác minh danh tính" with href "/seller/verification"
