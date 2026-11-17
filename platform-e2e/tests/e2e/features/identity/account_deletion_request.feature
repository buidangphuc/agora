@auth @buyer @account_deletion
Feature: A signed-in user schedules, inspects and cancels account deletion
  PrivacyService.RequestAccountDeletion needs the caller's password, schedules erasure 48 h out and
  GetAccountDeletionStatus and CancelAccountDeletion act on the caller's own state only
  (change identity-account-deletion).

  @deletion_default
  Scenario: Buyer with no obligations schedules deletion
    Given the stack runs with the default 48 hour cancel window
    And a signed-in buyer "A" with no obligations
    When "A" requests account deletion with the correct password
    Then the request succeeds with erase_after equal to the request time plus 48 hours within 5 seconds
    And the deletion status of "A" is pending with the same erase_after

  Scenario: Wrong password schedules nothing
    Given a signed-in buyer "A" with no obligations
    When "A" requests account deletion with the password "definitely-not-the-password"
    Then the call fails with code "permission_denied"
    And the deletion status of "A" is not pending

  Scenario: Empty password is rejected
    Given a signed-in buyer "A" with no obligations
    When "A" requests account deletion with an empty password
    Then the call fails with code "invalid_argument"
    And the deletion status of "A" is not pending

  Scenario: Anonymous caller is unauthenticated
    Given a signed-in buyer "A" with no obligations
    When a caller without a token requests account deletion through the gateway
    Then the call is rejected as unauthenticated
    And the deletion status of "A" is not pending

  Scenario: Service principal is denied
    Given a signed-in buyer "A" with no obligations
    When a service principal requests account deletion for "A" directly on team-identity
    Then the call fails with code "permission_denied"
    And the deletion status of "A" is not pending

  @deletion_default
  Scenario: Repeated request keeps the first deadline
    Given the stack runs with the default 48 hour cancel window
    And a signed-in buyer "A" with no obligations
    And "A" has requested account deletion
    When "A" requests account deletion with the correct password
    Then the request succeeds with the erase_after of the first request

  Scenario: Cancel within the window
    Given a signed-in buyer "A" with no obligations
    And "A" has requested account deletion
    When "A" cancels account deletion
    Then the deletion status of "A" is not pending
    And "A" can still sign in

  Scenario: Cancel with nothing pending is rejected
    Given a signed-in buyer "A" with no obligations
    When "A" cancels account deletion
    Then the call fails with code "failed_precondition"

  Scenario: Anonymous cancel is unauthenticated
    When a caller without a token cancels account deletion through the gateway
    Then the call is rejected as unauthenticated

  Scenario: A user cannot cancel another user's deletion
    Given a signed-in buyer "A" with no obligations
    And a signed-in buyer "B" with no obligations
    And "A" has requested account deletion
    When "B" cancels account deletion
    Then the call fails with code "failed_precondition"
    And the deletion status of "A" is pending with the same erase_after

  Scenario: Status with nothing pending
    Given a signed-in buyer "A" with no obligations
    When "A" reads the deletion status
    Then the status response has pending false and no erase_after

  Scenario: Status with pending deletion
    Given a signed-in buyer "A" with no obligations
    And "A" has requested account deletion
    When "A" reads the deletion status
    Then the status response has pending true and the erase_after of the first request

  Scenario: Anonymous status is unauthenticated
    When a caller without a token reads the deletion status through the gateway
    Then the call is rejected as unauthenticated

  Scenario: Pending deletion does not block sign-in
    Given a signed-in buyer "A" with no obligations
    And "A" has requested account deletion
    Then "A" can still sign in
