@auth @buyer @seller @account_deletion
Feature: Open obligations block account deletion with a named reason
  Open orders, a wallet balance and the like stop the request with FAILED_PRECONDITION naming the
  blocker category only (change identity-account-deletion).

  Scenario: Open order blocks deletion
    Given a signed-in buyer "A" with an open order
    When "A" requests account deletion with the correct password
    Then the call fails with code "failed_precondition"
    And the failure message names open orders
    And the deletion status of "A" is not pending

  Scenario: Non-zero wallet balance blocks deletion
    Given a signed-in seller "S" with a settled wallet balance
    When "S" requests account deletion with the correct password
    Then the call fails with code "failed_precondition"
    And the failure message names the wallet balance and contains no amount
    And the deletion status of "S" is not pending

  Scenario: Obligations are checked for the caller only
    Given a signed-in buyer "B" with an open order
    And a signed-in buyer "A" with no obligations
    When "A" requests account deletion with the correct password
    Then the request succeeds
    And the deletion status of "B" is not pending

  Scenario: Zero obligations on every counter allows deletion
    Given a signed-in seller "S" with no obligations
    When "S" requests account deletion with the correct password
    Then the request succeeds
    And the deletion status of "S" is pending with the same erase_after
