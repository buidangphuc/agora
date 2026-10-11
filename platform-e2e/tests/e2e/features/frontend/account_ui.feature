@buyer
Feature: Account and auth screens - forms, feedback, URL state and layout
  ui-phase-account: validated login/register forms, address-book mutations with feedback, KYC,
  referral, following, favorites and notifications. Needs the full stack (a signed-in buyer
  backed by the gateway); every assertion is on the rendered UI or on what the browser sent.

  # ── Shell and session ─────────────────────────────────────────────────
  Scenario: Anonymous visitor is redirected
    When an anonymous visitor opens "/account/addresses"
    Then they are redirected to the login page and no address data is in the response

  # ── Login and register ────────────────────────────────────────────────
  Scenario: Required fields are validated before submit
    Given the login page is open
    When the visitor submits the login form with an empty username
    Then no request is sent and the username field shows help, is invalid and has focus

  Scenario: Pending submit is visible and inert
    Given a registered buyer account
    And the login page is open
    And the form submission is slowed down
    When the visitor signs in with that account and presses the submit button twice
    Then the submit button shows a spinner, is busy and keeps its width
    And exactly one login request was sent

  Scenario: Register enforces the minimum lengths
    Given the register page is open
    When the visitor submits a 2-character username and a 3-character password
    Then both fields show their minimum-length help and no request is sent

  # ── Address book ──────────────────────────────────────────────────────
  Scenario: Adding an address
    Given a buyer is signed in on the addresses page
    And the form submission is slowed down
    When the buyer submits a valid address in the add modal
    Then the submit button is pending during the request
    And the modal closes, a success toast appears and the new address card is listed

  Scenario: A failed mutation reports an error and keeps state
    Given a signed-in buyer with a default and a second delivery address
    When the buyer opens the delivery addresses page
    And the second address is deleted behind the page's back
    And the buyer sets the second address as default
    Then an error toast is shown and the first address keeps its default tag
    And the set-default button is enabled again

  Scenario: Empty address book
    Given a buyer is signed in on the addresses page
    Then an Empty block says "Bạn chưa có địa chỉ nhận hàng nào." with the action "Thêm địa chỉ ngay"

  Scenario: Double submit is blocked
    Given a buyer is signed in on the addresses page
    And the form submission is slowed down
    When the buyer submits a valid address in the add modal and clicks the submit button twice
    Then exactly one mutation request was sent and one address was created

  Scenario: Action result shape
    Given a signed-in buyer with a default and a second delivery address
    When the buyer opens the delivery addresses page
    And the second address is deleted behind the page's back
    And the buyer sets the second address as default
    Then the action resolved without throwing and a readable error is shown, not the error page

  # ── Security ──────────────────────────────────────────────────────────
  Scenario: Empty history
    Given a signed-in buyer on the account security page
    Then the history table shows Empty "Chưa có lịch sử đăng nhập."

  Scenario: Revoking a session
    Given a buyer signed in on this device and on another device
    And the form submission is slowed down
    When the buyer confirms "Thu hồi" on the other device's session
    Then the confirm button is pending, a success toast appears and that row shows "Đã thu hồi" without a Revoke button

  # ── KYC ───────────────────────────────────────────────────────────────
  Scenario: Submitting a KYC document
    Given a signed-in buyer on the verification page
    When the buyer enters a reference and presses "Gửi hồ sơ xác minh"
    Then a success toast appears, the field is cleared and the status tag reads as pending

  Scenario: Status is not conveyed by colour alone
    Given a signed-in buyer on the verification page
    Then the status tag contains text that states the status

  Scenario: Status colours are semantic
    Given an admin has verified one buyer and rejected another
    Then the pending, verified and rejected status tags use the promo, success and danger tokens and not the brand colour

  # ── Referral ──────────────────────────────────────────────────────────
  Scenario: Redeeming an invalid code
    Given a signed-in buyer on the referral page
    When the buyer redeems the code "NOT-A-REAL-CODE"
    Then an error toast is shown and the redeem field shows the server message as help and is invalid

  Scenario: No rewards yet
    Given a signed-in buyer on the referral page
    Then the rewards area shows Empty "Chưa có phần thưởng nào."

  # ── Following ─────────────────────────────────────────────────────────
  Scenario: Tab is held in the URL
    Given a signed-in buyer
    When the buyer opens "/account/following?tab=items" with JavaScript disabled
    Then the "Sản phẩm" tab is selected on first render

  Scenario: Shop cards show the real shop name
    Given a buyer who follows a shop named "Cửa hàng Hoa Mai"
    When the buyer opens the following page
    Then the shop card shows "Cửa hàng Hoa Mai" and not "Shop #"

  Scenario: Empty shop name falls back
    Given a buyer who follows a shop with no display name
    When the buyer opens the following page
    Then the shop card shows "Shop #" and the first 6 characters of the shop id

  Scenario: Empty follow list
    Given a signed-in buyer
    When the buyer opens the following page
    Then the shops tab shows Empty "Bạn chưa theo dõi gian hàng nào." with a "Khám phá gian hàng" action linking to "/search"

  # ── Favorites ─────────────────────────────────────────────────────────
  Scenario: Collection filter in the URL
    Given a signed-in buyer on the favorites page
    And the buyer has a collection "Bo suu tap E2E" holding one favourite
    When the buyer selects that collection
    Then the URL is "/favorites?collection=<id>" and the header names the collection
    And reloading the page shows the same item

  Scenario: Collection management gives feedback
    Given a signed-in buyer on the favorites page
    And the form submission is slowed down
    When the buyer creates a collection "Danh sach moi"
    Then the create button is pending and disabled
    And a success toast appears and the new collection is listed

  Scenario: Favorites impressions and clicks still fire
    Given a signed-in buyer on the favorites page
    And the buyer has a favourite listing
    When the buyer opens the favorites page and clicks the favourite listing
    Then a view_item_list impression and a select_item click are sent for that listing

  # ── Notifications ─────────────────────────────────────────────────────
  Scenario: Tab survives reload and sharing
    Given a signed-in buyer on the notifications page
    When the buyer opens the "Đơn hàng" notification tab and reloads the page
    Then the URL contains "tab=order", the "Đơn hàng" notification tab is selected and only order notifications are listed

  Scenario: Empty tab
    Given a signed-in buyer on the notifications page
    Then the notifications list area shows Empty "Chưa có thông báo nào trong mục này."

  Scenario: No mark-all-read control
    Given a signed-in buyer on the notifications page
    Then no mark-all-read control is rendered

  Scenario: Saving preferences
    Given a signed-in buyer on the notifications page
    And the form submission is slowed down
    When the buyer toggles a preference and presses "Lưu tùy chọn"
    Then the save button is pending and then a success toast appears

  Scenario: Test hooks are preserved
    Given a buyer subscribed to a "price_drop" alert on a seeded listing
    When the seller lowers the listing price
    Then a "price_drop" notification appears in the notifications center
    And each notification and alert-subscription row exposes its data-testid and a data-type

  # ── Loading, layout and tokens ────────────────────────────────────────
  Scenario: Skeleton matches the content
    Given a signed-in buyer
    When the buyer loads "/account/security" over a throttled network
    Then the measured layout shift of the page is 0

  Scenario: No horizontal scroll on mobile
    Given a signed-in buyer
    When each of the nine account and auth routes is rendered at 375px wide
    Then none of them is wider than 375px

  Scenario: Desktop layout
    Given a signed-in buyer
    When the buyer opens "/account/addresses" at 1280px wide
    Then the 240px menu column is visible to the left of the content

  Scenario: Token lint is clean
    When the token lint runs on the account, auth, favorites and notifications files
    Then the token lint reports no violation in those files
