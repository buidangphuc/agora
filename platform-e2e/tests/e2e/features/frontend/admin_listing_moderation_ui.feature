@admin
Feature: Admin listing moderation queue in the browser (frontend-listing-moderation)
  /admin/listings is admin-only and lists flagged listings; the admin approves or rejects them and
  the decision fails closed when team-audit is down. Scenario names echo the spec scenarios of
  openspec/changes/archive/2026-10-18-frontend-listing-moderation. Every refusal is paired with a positive control
  (the admin sees the queue) so it cannot pass vacuously.

  Scenario: Moderation UI: Anonymous browser visit
    Given no one is logged in
    When the visitor opens the moderation queue
    Then the browser lands on the login page with a returnUrl for "/admin/listings"
    When an admin is signed in the browser
    And the visitor opens the moderation queue
    Then the moderation queue is shown

  Scenario: Moderation UI: A buyer opening the pages in the browser is refused
    Given a freshly registered moderation buyer
    And the moderation buyer is signed in the browser
    When the visitor opens the moderation queue
    Then the moderation page shows "Cần tài khoản Quản trị" and no table
    When an admin is signed in the browser
    And the visitor opens the moderation queue
    Then the moderation queue is shown

  Scenario: Moderation UI: A flagged listing appears in the queue end to end
    Given a moderation seller who published a clean and a flagged listing
    And an admin is signed in the browser
    When the visitor opens the moderation queue
    Then the queue lists the flagged listing with a screen flag
    And the queue does not list the clean listing

  Scenario: Moderation UI: Admin approves a flagged listing end to end
    Given a moderation seller who published a clean and a flagged listing
    And an admin is signed in the browser
    When the visitor opens the moderation queue
    And the admin presses "Duyệt" on the flagged listing
    Then the toast "Đã duyệt tin đăng." is shown
    And the flagged listing row leaves the queue
    And an anonymous visitor can open the flagged listing page

  Scenario: Moderation UI: Admin rejects a flagged listing and the seller sees the reason
    Given a moderation seller who published a clean and a flagged listing
    And an admin is signed in the browser
    When the visitor opens the moderation queue
    And the admin rejects the flagged listing with reason "Hàng giả" and the note "Ảnh sản phẩm là hàng nhái"
    Then the toast "Đã từ chối tin đăng." is shown
    And the flagged listing row leaves the queue
    And an anonymous reader gets not_found for the flagged listing
    When the moderation seller is signed in the browser
    And the seller opens the seller center
    Then the seller list shows the flagged listing with the tag "Bị từ chối"
    When the seller opens the edit page of the flagged listing
    Then the edit page shows the reason "Hàng giả" and the note "Ảnh sản phẩm là hàng nhái"

  @destructive
  Scenario: Moderation UI: Moderation fails closed when audit is down
    Given a moderation seller who published a clean and a flagged listing
    And an admin is signed in the browser
    And team-audit is down for moderation
    When the visitor opens the moderation queue
    And the admin presses "Duyệt" on the flagged listing
    Then the page shows "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau."
    When team-audit is up again for moderation
    And the visitor opens the moderation queue
    Then the queue lists the flagged listing with a screen flag
    And the flagged listing is still pending review at the gateway
