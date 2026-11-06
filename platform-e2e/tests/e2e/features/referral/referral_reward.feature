@order
Feature: Referral attribution and delayed rewards
  An invited signup is attributed asynchronously, the invitee's first completed order creates
  a pending reward, and the sweeper releases it once the return window has passed.

  Background:
    Given a seller with a stocked listing for referral orders
    And the referral events topic is being observed
    And an inviter "alice" with a referral code

  Scenario: An invited signup is attributed and the first order creates a pending reward
    When "bob" registers through the gateway with the code of "alice"
    Then ReferralAttributed for "bob" naming "alice" is observed within 10 seconds
    And "alice" shows invited_count 1
    When "bob" completes an order
    Then "alice" shows pending_xu 10000 and released_xu 0
    And "alice" lists exactly one PENDING reward of 10000 xu

  Scenario: A reward is released after the return window
    Given the stack sweeps releases every second and closes the return window after 3 seconds
    And "bob" registered through the gateway with the code of "alice"
    When "bob" completes an order
    Then "alice" shows released_xu 10000 within the return window plus 2 seconds
    And "alice" shows pending_xu 0 and released_xu 10000
    And "alice" shows rewards_total 10000
    And "alice" lists exactly one RELEASED reward of 10000 xu
    And ReferralRewardReleased of 10000 xu for "bob" naming "alice" is observed

  Scenario: A return before release voids the reward and the next order qualifies
    Given "bob" registered through the gateway with the code of "alice"
    And "bob" completed an order
    And "alice" shows pending_xu 10000 and released_xu 0
    When "bob" requests a return on that order
    Then "alice" shows pending_xu 0 and released_xu 0
    And "alice" lists no reward
    When "bob" completes an order
    Then "alice" shows pending_xu 10000 and released_xu 0
    And "alice" lists exactly one PENDING reward of 10000 xu

  Scenario: Manual redeem is accepted for a new account and credits nothing
    Given "carol" registered through the gateway with no referral code
    When "carol" redeems the code of "alice" once the registration is known
    Then the redeem is accepted
    And "alice" shows invited_count 1
    And "alice" shows pending_xu 0 and released_xu 0
    And "alice" lists no reward

  Scenario: Manual redeem is refused for a buyer with a completed order
    Given "erin" registered through the gateway with no referral code
    And "erin" completed an order and the order events were consumed
    When "erin" redeems the code of "alice"
    Then the redeem is refused with FAILED_PRECONDITION and a message
    And "alice" shows invited_count 0
    And no ReferralAttributed for "erin" is observed

  Scenario: The daily cap defers rewards
    Given the stack sweeps releases every second, closes the return window after 3 seconds and caps releases at 2 per day
    When three invitees "b1", "b2" and "b3" register with the code of "alice"
    And the three invitees each complete an order
    Then "alice" ends with two released rewards and one pending reward after the release sweeps
