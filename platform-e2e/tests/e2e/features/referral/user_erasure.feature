@order
Feature: team-referral deletes a user's referral identity on UserErased
  A UserErased event on identity.events deletes the inviter's code, account and first-order rows
  and keeps the reward ledger. A deleted code no longer attributes a new registration.
  (change commerce-user-erasure)

  Scenario: REF-1 Code, account and first-order rows are deleted and the ledger is kept
    Given a seller with a stocked listing for referral orders
    And an inviter "I" with a referral code and an invitee "R1" who registered with it and completed a first order
    When a UserErased event for "I" is consumed
    And a new user "R2" registers through the gateway with the code of "I" after its erasure
    Then "R2" is not attributed to "I"
    And the rewards of "I" are still listed with the same ids and amounts

  Scenario: REF-2 A deleted code no longer resolves
    Given a seller with a stocked listing for referral orders
    And an inviter "I" with a referral code and an invitee "R1" who registered with it and completed a first order
    When a UserErased event for "I" is consumed
    And a new user "R2" registers through the gateway with the code of "I" after its erasure
    Then "R2" is not attributed to "I"
