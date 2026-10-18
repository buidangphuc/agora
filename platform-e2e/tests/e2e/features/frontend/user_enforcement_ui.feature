@admin
Feature: Admin users page and login enforcement in the browser (frontend-user-enforcement)
  /admin/users lets an admin find a user and set a status; a suspended or banned user is refused
  at /login with one fixed text; a restricted seller sees a notice instead of a login loop.
  Scenario names echo the spec scenarios of openspec/changes/archive/2026-10-18-frontend-user-enforcement. Every
  refusal is paired with a positive control (the admin sees the page, the reinstated user logs in,
  the stored status is read back through the gateway) so it cannot pass vacuously.

  Scenario: Users UI: Anonymous browser visit
    Given no one is logged in
    When the visitor opens the users page
    Then the browser lands on the login page with a returnUrl for the users page

  Scenario: Users UI: A buyer opening the page in the browser is refused
    Given I am logged in as a buyer via API
    When the visitor opens the users page
    Then the users page shows "Cần tài khoản Quản trị" and no user table
    Given an admin is logged in
    When the visitor opens the users page
    Then the users table is shown

  Scenario: Users UI: Admin finds a user by username in the browser
    Given a freshly registered buyer for the users page
    And an admin is logged in
    When the admin opens the users page
    And the admin searches for that buyer's username
    Then the table shows that buyer with status "Hoạt động"

  Scenario: Users UI: Admin suspends a buyer from the users page
    Given a freshly registered buyer for the users page
    And an admin is logged in
    When the admin opens the users page
    And the admin searches for that buyer's username
    And the admin submits "Tạm khóa" with reason "fraud" and an end time one hour ahead
    Then the users page shows the success message
    And the buyer's row reads "Tạm khóa" with that end time
    And the gateway reports the buyer as "suspended"

  Scenario: Users UI: Admin bans then reinstates a buyer
    Given a freshly registered buyer for the users page
    And an admin is logged in
    When the admin opens the users page
    And the admin searches for that buyer's username
    And the admin submits "Cấm" with reason "abuse" and no end time
    Then the buyer's row reads "Cấm"
    And the gateway reports the buyer as "banned"
    When the admin submits "Hoạt động" with reason "other" and no end time
    Then the buyer's row reads "Hoạt động"
    And the gateway reports the buyer as "active"

  Scenario: Users UI: Admin has no controls on their own row
    Given a freshly registered buyer for the users page
    And an admin is logged in
    When the admin opens the users page
    And the admin searches for that buyer's username
    Then the buyer's row has a status form
    When the admin searches for their own username
    Then the admin's own row has no status form

  @destructive
  Scenario: Users UI: Audit down in the browser leaves the user active
    Given a freshly registered buyer for the users page
    And an admin is logged in
    And team-audit is down for this scenario
    When the admin opens the users page
    And the admin searches for that buyer's username
    And the admin submits "Tạm khóa" with reason "fraud" and an end time one hour ahead
    Then the users page shows "Hệ thống chưa ghi nhận được thao tác; trạng thái không thay đổi. Vui lòng thử lại sau."
    And the gateway reports the buyer as "active"
    And the buyer can still log in through the gateway

  Scenario: Users UI: Suspended buyer cannot log in
    Given a freshly registered buyer for the users page
    And an admin has set that buyer to "suspended"
    When the buyer submits the correct password on the login page
    Then the login page shows the fixed refusal text
    And the browser is still on the login page

  Scenario: Users UI: Reinstated user can log in again
    Given a freshly registered buyer for the users page
    And an admin is logged in
    When the admin opens the users page
    And the admin searches for that buyer's username
    And the admin submits "Cấm" with reason "abuse" and no end time
    Then the buyer's row reads "Cấm"
    When the admin submits "Hoạt động" with reason "other" and no end time
    Then the buyer's row reads "Hoạt động"
    When the buyer submits the correct password on the login page
    Then the browser leaves the login page
    And the header shows the signed-in state

  Scenario: Users UI: Wrong password for a suspended buyer shows the credentials message
    Given a freshly registered buyer for the users page
    And an admin has set that buyer to "suspended"
    When the buyer submits a wrong password on the login page
    Then the login page shows "Tên đăng nhập hoặc mật khẩu không chính xác." and not the refusal text
    When the buyer submits the correct password on the login page
    Then the login page shows the fixed refusal text

  Scenario: Users UI: Restricted seller sees the notice in the browser
    Given a freshly registered seller for the users page
    And an admin has set that seller to "restricted"
    When the seller logs in on the login page and opens "/seller/new"
    Then the browser is still on "/seller/new"
    And the seller notice shows "Cần tài khoản Người Bán" and "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục."
