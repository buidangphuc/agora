@seller @fintech @seller_kyc_ui
Feature: Seller wallet explains the verification requirement for payouts
  The wallet page shows a notice while the seller is not VERIFIED and maps the gateway's payout
  refusals to Vietnamese messages. Verification state and balance come from the real KYC and payment
  RPCs; the outage case stops team-verification for real.
  (seller-kyc-ui)

  @needsSeller
  Scenario: Wallet shows a notice while not verified
    Given a seeded seller is logged in
    And the seller has no KYC submission
    When the seller opens the seller wallet page
    Then the wallet shows the notice "Cần xác minh danh tính để rút tiền" with a link to "/seller/verification"

  @needsSeller @needsOrder
  Scenario: A refused payout shows the verification message
    Given a seeded seller is logged in
    And the seller has no KYC submission
    And the buyer has paid the seeded order to the seller
    When the seller opens the seller wallet page
    And the seller confirms a payout of the offered amount
    Then the payout dialog shows "Cần xác minh danh tính để rút tiền" with a link to "/seller/verification"
    And the seller's ledger balance is unchanged
    And the payout dialog does not show "seller verification required"

  @needsSeller @needsOrder @destructive
  Scenario: Verification outage during payout shows a retry message
    Given a seeded seller is logged in
    And the seller was approved by an admin through ReviewKyc
    And the buyer has paid the seeded order to the seller
    When the seller opens the seller wallet page
    And the seller opens the payout dialog
    And team-verification is stopped for the seller UI
    And the seller confirms a payout of the offered amount
    Then the payout dialog shows "Chưa kiểm tra được trạng thái xác minh, vui lòng thử lại sau"
    And the payout confirm button is enabled again

  @needsSeller @needsOrder
  Scenario: A verified seller sees no notice and can pay out
    Given a seeded seller is logged in
    And the seller was approved by an admin through ReviewKyc
    And the buyer has paid the seeded order to the seller
    When the seller opens the seller wallet page
    Then the wallet shows no verification notice
    When the seller confirms a payout of the offered amount
    Then the toast "Đã tạo lệnh rút tiền" is shown
