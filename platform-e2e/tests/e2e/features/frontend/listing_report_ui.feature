@buyer
Feature: Reporting a listing from its page (frontend-listing-moderation)
  A guest is sent to login and back, a buyer's report reaches the admin inbox, and the seller never
  sees who reported. Scenario names echo the spec scenarios of
  openspec/changes/archive/2026-10-18-frontend-listing-moderation. Every negative check is paired with a positive one
  (the control is shown after login, the report exists at the gateway).

  Scenario: Moderation UI: Guest is sent to login and back
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And no one is logged in
    When the visitor opens the reported listing page
    And the visitor clicks "Báo cáo tin đăng"
    Then the browser lands on the login page with a returnUrl for the reported listing
    When the moderation buyer signs in on that login page
    Then the browser is on the reported listing page
    And the reported listing page shows the "Báo cáo tin đăng" button

  Scenario: Moderation UI: A buyer's report reaches the admin inbox
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the moderation buyer is signed in the browser
    When the moderation buyer reports the listing in the browser with reason "Lừa đảo" and the note "Giá quá rẻ, yêu cầu chuyển khoản ngoài"
    Then the toast "Đã gửi báo cáo. Cảm ơn bạn." is shown
    And the gateway lists one open scam report on the listing by the buyer
    When an admin is signed in the browser
    And the admin opens the cases inbox
    Then the open reports show a row for the listing with "Lừa đảo" and the buyer as reporter

  Scenario: Moderation UI: The seller never sees the reporter
    Given a moderation seller with a published clean listing
    And a freshly registered moderation buyer
    And the engagement service knows the moderation listing
    And the buyer has reported the listing through the gateway with the note "Báo cáo bí mật của người mua"
    And the moderation seller is signed in the browser
    When the seller opens the seller center
    Then the seller list shows the listing
    And the page text hides the reporter and the report note
    When the seller opens the edit page of the listing
    Then the edit page shows the listing title
    And the page text hides the reporter and the report note
