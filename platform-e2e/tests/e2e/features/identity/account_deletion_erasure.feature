@auth @buyer @account_deletion @slow @deletion_sweep_override
Feature: The sweeper erases an account after erase_after and publishes UserErased
  Short-window stack: compose/account-deletion.override.yaml on team-identity
  (ACCOUNT_DELETION_CANCEL_WINDOW=20s, ACCOUNT_ERASURE_SWEEP_INTERVAL=2s).
  Identity data is anonymised or deleted, proof rows stay, one UserErased reaches identity.events and
  the former credentials stop working (change identity-account-deletion).

  Scenario: Due account is erased
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    Then the user row of "U" has status deleted, a deleted_ and 16 hex username, an empty password hash and no roles
    And no addresses, sessions, login history, handoff codes or reset tokens remain for "U"

  Scenario: UserErased is written in the same transaction
    Given the stack runs with the short deletion window override
    And a signed-in seller "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    Then exactly one UserErased event for "U" is on identity.events carrying the roles held before erasure
    And the user row of "U" has status deleted

  Scenario: Consent and seller-terms proof survive erasure
    Given the stack runs with the short deletion window override
    And a signed-in seller "U" with an address and two sessions
    And the consent record count of "U" is recorded
    When "U" requests account deletion with the correct password and the window passes
    Then the consent record count of "U" is unchanged and at least 2

  Scenario: Account not yet due is untouched
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password
    And 6 seconds pass inside the window
    Then the user row of "U" is still active with its address and sessions
    And no UserErased event for "U" is on identity.events

  Scenario: Cancelled account is untouched
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password
    And "U" cancels account deletion
    And the former erase_after has passed by two sweep intervals
    Then the user row of "U" is still active with its address and sessions
    And no UserErased event for "U" is on identity.events

  Scenario: Sweep is idempotent
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    And three more sweep intervals pass
    Then exactly one UserErased event for "U" is on identity.events

  Scenario: Erasure completes within 72 h of the request
    Given the stack runs with the short deletion window override
    And a signed-in seller "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    Then the account of "U" was erased no later than the window plus two sweep intervals after the request
    And a UserErased event for "U" is on identity.events within the same bound

  Scenario: Login to an erased account fails
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    And someone logs in with the former username and password of "U"
    And someone logs in with an unknown username
    Then both logins fail with the same code and message

  Scenario: Former username is not reusable by lookup
    Given the stack runs with the short deletion window override
    And a signed-in buyer "U" with an address and two sessions
    When "U" requests account deletion with the correct password and the window passes
    And a visitor registers the former username of "U"
    Then the registration succeeds
