@promo
Feature: Every new account receives exactly one new-user voucher
  team-promotion consumes UserRegistered and the referral verdicts and issues a welcome or an
  invited voucher to the registered user, polled through ListMyVouchers every 500 ms.

  Scenario: Registration without a referral code issues the welcome voucher
    When "alice" registers through the gateway with no referral code
    Then "alice" owns exactly one new-user voucher with discount 20000 within 10 seconds

  Scenario: Invited signup waits for the verdict and gets the invited voucher
    Given the referral events topic is being observed for new-user vouchers
    And an inviter "inviter" with a referral code for new-user vouchers
    When "alice" registers through the gateway with the code of "inviter"
    Then ReferralAttributed for "alice" is observed within 10 seconds
    And "alice" owns exactly one new-user voucher with discount 30000 within 10 seconds

  Scenario: A rejected referral falls back to the welcome voucher
    Given the referral events topic is being observed for new-user vouchers
    When "alice" registers through the gateway with an unknown well-formed referral code
    Then ReferralRejected for "alice" is observed within 10 seconds
    And "alice" owns exactly one new-user voucher with discount 20000 within 10 seconds

  @destructive
  Scenario: Missing verdict falls back to the welcome voucher after the timeout
    Given the stack uses a 2 second verdict timeout and a 1 second sweep
    And the referral events topic is being observed for new-user vouchers
    And an inviter "inviter" with a referral code for new-user vouchers
    And team-referral is stopped
    When "alice" registers through the gateway with the code of "inviter"
    Then "alice" owns no voucher yet
    And "alice" owns exactly one new-user voucher with discount 20000 within the verdict timeout plus two sweeps
    When team-referral is started again
    Then ReferralAttributed for "alice" is observed after the restart
    And "alice" still owns exactly one new-user voucher with discount 20000
