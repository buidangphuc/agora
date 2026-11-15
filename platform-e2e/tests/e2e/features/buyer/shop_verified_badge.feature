@buyer @seller_kyc_ui
Feature: KYC lives only in the seller app and the shop page shows a public verified badge
  The buyer app serves no /account/verification and links nowhere to it. /shop/[id] shows a
  "Đã xác minh" badge if and only if the seller is VERIFIED and exposes no other verification state.
  Sellers become VERIFIED only through the real KYC RPCs; the lookup failure stops team-verification.
  (seller-kyc-ui)

  @needsBuyer
  Scenario: The buyer route is gone
    Given a buyer is logged in
    When the buyer requests "/account/verification" on the buyer app
    Then the response status is 404

  @needsBuyer
  Scenario: Buyer navigation has no verification link
    Given a buyer is logged in
    When the buyer opens the account pages "/account", "/account/security" and "/account/addresses"
    Then every page has 0 links to "/account/verification" in the account nav and the top bar

  @needsSeller
  Scenario: Verified seller's shop shows the badge
    Given the seller was approved by an admin through ReviewKyc
    When an anonymous visitor opens the shop page of the seller
    Then the shop page contains exactly 1 "Đã xác minh" badge

  @needsSeller
  Scenario Outline: Non-verified shops show no badge and no status
    Given the seller's KYC state is <state>
    When an anonymous visitor opens the shop page of the seller
    Then the shop page contains 0 "Đã xác minh" badges
    And the shop page shows none of "Đang chờ duyệt", "Bị từ chối" and "Chưa gửi hồ sơ"

    Examples:
      | state         |
      | PENDING       |
      | REJECTED      |
      | NOT_SUBMITTED |

  @needsSeller @destructive
  Scenario: Badge lookup failure does not break the page
    Given the seller was approved by an admin through ReviewKyc
    And team-verification is stopped for the seller UI
    When an anonymous visitor opens the shop page of the seller
    Then the shop page answers with HTTP 200 and contains 0 "Đã xác minh" badges
