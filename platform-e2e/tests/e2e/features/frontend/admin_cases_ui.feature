@admin
Feature: Admin cases inbox in the browser (frontend-listing-moderation)
  /admin/cases lists reports; the admin bans or dismisses and closes them, closing fails closed
  when team-audit is down, and the pages are reachable from the cockpit. Scenario names echo the
  spec scenarios of openspec/changes/frontend-listing-moderation. Every refusal is paired with a
  positive control (the admin sees the inbox) so it cannot pass vacuously.

  Scenario: Moderation UI: Admin bans a reported listing and closes the report
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the buyer has reported the listing through the gateway with the note "Hàng nhái"
    And an admin is signed in the browser
    When the admin opens the cases inbox
    And the admin starts "Xử lý tin đăng" on the report with decision "Khóa" and reason "Hàng giả"
    And the admin closes the report as "Đã xử lý" with the note "Đã khóa tin đăng"
    Then the toast "Đã đóng báo cáo." is shown
    And an anonymous reader gets not_found for the reported listing
    And the report is listed under "Đã xử lý" and no longer under "Đang mở"

  Scenario: Moderation UI: Admin dismisses a report
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the buyer has reported the listing through the gateway with the note "Báo cáo sai"
    And an admin is signed in the browser
    When the admin opens the cases inbox
    And the admin closes the report as "Bác bỏ" with the note "Không vi phạm"
    Then the toast "Đã đóng báo cáo." is shown
    And the report is listed under "Đã bác bỏ" and no longer under "Đang mở"
    And an anonymous visitor can open the reported listing page

  @destructive
  Scenario: Moderation UI: Closing a report fails closed when audit is down
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the buyer has reported the listing through the gateway with the note "Báo cáo thử"
    And an admin is signed in the browser
    And team-audit is down for moderation
    When the admin opens the cases inbox
    And the admin closes the report as "Bác bỏ" with the note "Không vi phạm"
    Then the page shows "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau."
    When team-audit is up again for moderation
    And the admin opens the cases inbox
    Then the open reports show a row for the listing

  Scenario: Moderation UI: Admin reaches the pages from the cockpit
    Given an admin is signed in the browser
    When the admin opens the cockpit
    And the admin follows the link "Hộp thư vụ việc"
    Then the browser is on "/admin/cases"
    And the "Báo cáo" tab is selected

  Scenario: Moderation UI: A non-admin following an admin link is refused
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the buyer has reported the listing through the gateway with the note "Báo cáo kiểm tra"
    And the moderation buyer is signed in the browser
    When the buyer follows the footer link "Hộp thư vụ việc"
    Then the browser is on "/admin/cases"
    And the moderation page shows "Cần tài khoản Quản trị" and no table
    When an admin is signed in the browser
    And the admin opens the cases inbox
    Then the open reports show a row for the listing
