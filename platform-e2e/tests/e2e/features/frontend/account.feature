@buyer
Feature: Account screens
  The account and auth screens (ui-phase-account) share a URL-driven settings shell, hold view
  state in the URL and give inline, accessible feedback. They need the full stack (a logged-in
  buyer backed by the gateway), so these scenarios run against the e2e docker stack.

  Scenario: The menu reflects the URL
    Given a signed-in buyer on the account security page
    Then the account menu marks "Bảo mật" as the current page
    And the account page heading is "Bảo mật tài khoản"

  Scenario: Menu navigation is a real navigation
    Given a signed-in buyer on the account security page
    When the buyer opens "Xác minh" from the account menu
    Then the account menu marks "Xác minh" as the current page
    When the buyer goes back in the browser
    Then the account menu marks "Bảo mật" as the current page

  Scenario: Menu collapses on mobile
    Given a signed-in buyer on the account security page
    When the viewport is 375 pixels wide
    Then the account menu sits above the page content
    And the page content does not scroll sideways
    And the whole document does not scroll sideways

  Scenario: Delete requires confirmation
    Given a signed-in buyer with a default and a second delivery address
    When the buyer opens the delivery addresses page
    And the buyer asks to delete the second address
    Then a confirmation dialog names the recipient of the second address
    When the buyer presses Escape
    Then the confirmation dialog is closed and both addresses are still listed
    When the buyer asks to delete the second address
    And the buyer confirms the deletion
    Then the second address is no longer listed

  Scenario: Every KYC control has an accessible name
    Given a signed-in buyer on the verification page
    Then the document type select is named "Loại giấy tờ"
    And the document reference input is named "Mã tham chiếu tài liệu"
    And the submit button is disabled until a reference is typed

  Scenario: The notification tab survives a reload
    Given a signed-in buyer on the notifications page
    When the buyer selects the "Đơn hàng" notification tab
    Then the address bar shows the order tab
    When the buyer reloads the page
    Then the "Đơn hàng" notification tab is still the selected one

  Scenario: Unknown collection
    Given a signed-in buyer on the favorites page for the collection "does-not-exist"
    Then a not-found result offers "Xem tất cả sản phẩm yêu thích"

  Scenario: Wrong credentials show an inline error and keep the typed username
    Given the login page is open
    When the user logs in with username "khong_ton_tai" and password "saibet123"
    Then an inline error alert reads "Tên đăng nhập hoặc mật khẩu không chính xác."
    And the user is still on the login page with the username kept and the password cleared
