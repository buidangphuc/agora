@buyer
Feature: Welcome banner, activation checklist and own vouchers (frontend-welcome-checklist)
  A newly registered buyer sees the NEW_USER voucher in a banner and a three-step checklist on / and
  /account/* for 14 days, finds the voucher on /vouchers and in the checkout picker, and can dismiss
  each part. Account age is shifted by updating created_at of the buyer's own identity row; the
  voucher is read back through VoucherService.ListMyVouchers, the same read the pages render.
  Scenarios that need a failing or stalled upstream read, a buyer with no NEW_USER voucher, or an
  expired voucher are verified by the Vitest tests named in team-frontend/FEATURES.yaml.

  Scenario: W1 Own vouchers listed above the platform list
    Given a welcome buyer holding a NEW_USER voucher is signed in
    And the platform voucher "SAVE10" exists
    When the welcome buyer opens the vouchers page
    Then the page has a "Voucher của bạn" section containing the voucher code, its amount and its expiry
    And the section comes before the platform voucher "SAVE10"

  Scenario: W4 Anonymous visitor
    Given the platform voucher "SAVE10" exists
    When an anonymous visitor opens the vouchers page
    Then the platform voucher "SAVE10" is listed
    And the page has no "Voucher của bạn" section

  Scenario: W5 Another buyer's voucher is never shown (permission)
    Given a welcome buyer holding a NEW_USER voucher is signed in
    And a second welcome buyer holding a NEW_USER voucher
    When the welcome buyer opens the vouchers page
    Then the page shows the welcome buyer's voucher code
    And the page does not show the second buyer's voucher code
    When the second buyer opens the vouchers page
    Then the page shows the second buyer's voucher code
    And the page does not show the welcome buyer's voucher code

  Scenario: W6 Own voucher offered and applied
    Given a welcome buyer holding a NEW_USER voucher is signed in with a qualifying cart and an address
    When the welcome buyer opens the payment step
    And the welcome buyer picks the voucher from the picker
    Then the voucher code is marked applied in the platform slot
    And the checkout quote carries the voucher code as the platform voucher
    And the discount row shows a discount

  Scenario: W22 Anonymous checkout visitor (permission)
    When an anonymous visitor opens the checkout page
    Then the visitor is on the login page

  Scenario: W8 Ineligible voucher is refused by the quote
    Given a welcome buyer holding a NEW_USER voucher is signed in with a cart below the voucher minimum spend and an address
    When the welcome buyer opens the payment step
    And the welcome buyer picks the voucher from the picker
    Then the checkout shows an invalid-voucher state
    And the voucher code is not marked applied in the platform slot

  Scenario: W9 Banner shows the voucher
    Given a welcome buyer holding a NEW_USER voucher is signed in
    And the welcome buyer's account is 2 days old
    When the welcome buyer opens the home page
    Then the welcome banner shows the voucher code, the amount in đ and the expiry date

  Scenario: W10 Voucher not yet issued
    Given a freshly registered welcome buyer is signed in
    When the welcome buyer opens the addresses page
    Then the welcome banner shows the pending text without a code until the voucher exists
    And within 10 seconds the welcome banner shows the voucher code

  Scenario: W12 Account age boundary
    Given a welcome buyer holding a NEW_USER voucher is signed in
    And the welcome buyer's account is 13 days and 23 hours old
    When the welcome buyer opens the home page
    Then the welcome panel is rendered
    When the welcome buyer's account becomes 14 days old
    And the welcome buyer opens the home page
    Then the welcome panel is not rendered

  Scenario: W14 Dismiss the banner
    Given a welcome buyer holding a NEW_USER voucher is signed in
    When the welcome buyer opens the home page
    And the welcome buyer closes the welcome banner
    Then the cookie "agora_welcome_dismissed" is set with value 1 and the session attributes
    When the welcome buyer opens the home page
    Then the welcome banner is absent
    When the welcome buyer opens the addresses page
    Then the welcome banner is absent

  Scenario: W15 Anonymous visitor sees nothing (permission)
    When an anonymous visitor opens the home page
    Then the welcome panel is not rendered
    And the text "Voucher đang được chuẩn bị" is not shown

  Scenario: W16 Steps ticked from the reads
    Given a welcome buyer without address or order is signed in
    And the welcome buyer follows a shop
    When the welcome buyer opens the home page
    Then the checklist step "Theo dõi một shop" is done
    And the checklist step "Thêm địa chỉ giao hàng" is not done
    And the checklist step "Đặt đơn đầu tiên" is not done

  Scenario: W17 All done hides the checklist
    Given a welcome buyer with an address is signed in
    And the welcome buyer follows a shop
    When the welcome buyer opens the home page
    Then the checklist step "Đặt đơn đầu tiên" is not done
    When the welcome buyer places an order
    And the welcome buyer opens the home page
    Then the welcome checklist is absent

  Scenario: W19 Dismiss the checklist
    Given a welcome buyer holding a NEW_USER voucher is signed in
    When the welcome buyer opens the home page
    And the welcome buyer hides the checklist
    Then the cookie "agora_checklist_dismissed" is set with value 1 and the session attributes
    When the welcome buyer opens the home page
    Then the welcome checklist is absent
    And the welcome banner shows the voucher code

  Scenario: W20 Account older than 14 days
    Given a welcome buyer holding a NEW_USER voucher is signed in
    And the welcome buyer's account is 30 days old
    When the welcome buyer opens the home page
    Then the welcome panel is not rendered
    When the welcome buyer opens the addresses page
    Then the welcome panel is not rendered

  Scenario: W23 Anonymous visitor on an account page (permission)
    When an anonymous visitor opens the following page
    Then the visitor is on the login page
    And the welcome panel is not rendered

  Scenario: W25 Cancelled order does not tick the first-order step
    Given a welcome buyer with an address is signed in
    And the welcome buyer has only a cancelled order
    When the welcome buyer opens the home page
    Then the checklist step "Đặt đơn đầu tiên" is not done
    And the checklist step "Thêm địa chỉ giao hàng" is done
