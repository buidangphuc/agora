@auth @buyer @account_deletion
Feature: Scheduling deletion revokes the other sessions
  Other sessions of the user are revoked in the same transaction as the schedule, the calling session
  stays; a refused request revokes nothing (change identity-account-deletion).

  Scenario: Other sessions are revoked, current stays
    Given a signed-in buyer "A" with three active sessions
    When "A" requests account deletion from the first session
    Then the first session stays active and the other two are revoked
    And refreshing with the second session's refresh token fails with code "unauthenticated"

  Scenario: Failed scheduling revokes nothing
    Given a signed-in buyer "A" with an open order and three active sessions
    When "A" requests account deletion from the first session
    Then the call fails with code "failed_precondition"
    And all three sessions are still active
