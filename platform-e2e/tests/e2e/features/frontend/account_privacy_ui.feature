@buyer @account_privacy_ui
Feature: Account privacy page (change frontend-account-privacy)
  A signed-in user sees and changes their consent with a server record, downloads their data and
  requests or cancels account deletion from /account/privacy. Real PrivacyService RPCs through the
  gateway; nothing is mocked.

  Scenario: PV1 Signed-in user reaches the page from the account menu
    Given a signed-in privacy buyer on the account security page
    When the buyer opens "Quyền riêng tư" from the privacy account menu
    Then the browser is on "/account/privacy" with the heading "Quyền riêng tư"
    And the privacy account menu marks "Quyền riêng tư" as the current page

  Scenario: PV2 Guest is sent to login
    Given a privacy visitor with no session
    When the visitor opens the privacy page
    Then the browser is redirected to "/login" with the return URL "/account/privacy"
    And the gateway received 0 PrivacyService calls from the privacy visitor

  Scenario: PV4 History is listed newest first and paged
    Given a signed-in privacy buyer with 25 consent records whose newest is PERSONALIZATION
    When the buyer opens the privacy page
    Then the consent history shows 20 rows and the first row is the newest record
    And a pagination link to page 2 is shown
    When the buyer opens page 2 of the privacy history
    Then the consent history shows 5 rows

  Scenario: PV5 Saving a change records one row per changed purpose
    Given a signed-in privacy buyer with analytics granted and the consent cookie "a=1&m=0&p=0"
    When the buyer opens the privacy page
    And the buyer switches "Phân tích" off and "Tiếp thị" on and saves
    Then the toast "Đã lưu lựa chọn của bạn." is shown
    And the privacy consent cookie holds "a=0&m=1"
    And the gateway received 1 RecordConsent call from the privacy buyer
    And the ledger gained exactly 2 account rows for ANALYTICS withdrawn and MARKETING granted at version "1"
    And the consent history shows the 2 new records

  Scenario: PV11 Export downloads one JSON file
    Given a signed-in privacy buyer
    When the buyer opens the privacy page
    And the buyer clicks "Tải dữ liệu của tôi"
    Then the browser receives one file named "agora-du-lieu-<date>.json" that is valid JSON
    And the downloaded JSON has account.user_id equal to the buyer id

  Scenario: PV16 Confirming with the password schedules deletion
    Given the privacy stack runs the default 48 hour cancel window
    And a signed-in privacy buyer with no obligations
    When the buyer opens the privacy page
    And the buyer opens the deletion dialog and confirms with the correct password
    Then the pending deletion panel shows with the "Hủy yêu cầu xóa" button
    And the stored erase time is the deletion window after the request within 5 seconds

  Scenario: PV17 Wrong password schedules nothing
    Given a signed-in privacy buyer with no obligations
    When the buyer opens the privacy page
    And the buyer opens the deletion dialog and confirms with the password "WrongPassword-123"
    Then the deletion dialog stays open with "Mật khẩu không đúng." and an empty password field
    And no deletion is pending for the buyer

  Scenario: PV19 An open obligation shows the reason
    Given a signed-in privacy buyer with an open order
    When the buyer opens the privacy page
    And the buyer opens the deletion dialog and confirms with the correct password
    Then the deletion dialog shows "Chưa thể xóa tài khoản: " followed by a reason naming orders
    And no pending deletion panel is shown
    And no deletion is pending for the buyer
    And the buyer account is still usable

  Scenario: PV21 Pending deletion can be cancelled
    Given a signed-in privacy buyer with a pending deletion
    When the buyer opens the privacy page
    Then the pending deletion panel shows with the "Hủy yêu cầu xóa" button
    And the deletion offer "Xóa tài khoản" is not shown
    When the buyer cancels the deletion request and confirms
    Then the toast "Đã hủy yêu cầu xóa tài khoản." is shown
    And the deletion offer "Xóa tài khoản" is shown again
    And the gateway received 1 CancelAccountDeletion call from the privacy buyer
    And no deletion is pending for the buyer

  Scenario: PV26 A call without a session is refused
    Given a signed-in privacy buyer with the consent cookie "a=0&m=0&p=0"
    When the buyer opens the privacy page
    And the buyer's session token becomes invalid in the browser
    And the buyer switches "Phân tích" on and saves
    Then the privacy page shows "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại."
    And the privacy consent cookie holds "a=0&m=0&p=0"
