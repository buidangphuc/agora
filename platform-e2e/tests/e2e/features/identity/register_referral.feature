@auth
Feature: Register carries a referral code and announces the new user
  Register accepts an optional bounded referral code that never changes the response, and
  every successful registration publishes one UserRegistered event on identity.events.

  Scenario: Register with a well-formed code succeeds
    When a visitor registers a new username with referral code "AB12cd_-"
    Then Register succeeds with the same response shape as without a code
    And the UserRegistered event for the new user carries referral code "AB12cd_-"

  Scenario: Register without a code succeeds
    When a visitor registers a new username with no referral code
    Then Register succeeds
    And the UserRegistered event for the new user carries an empty referral code

  Scenario: A malformed code is dropped, not rejected
    When a visitor registers a new username with referral code "bad code;DROP TABLE"
    Then Register succeeds with the same response shape as without a code
    And the UserRegistered event for the new user carries an empty referral code

  Scenario: Surrounding whitespace is trimmed before validation
    When a visitor registers a new username with referral code " AB12 "
    Then Register succeeds
    And the UserRegistered event for the new user carries referral code "AB12"

  Scenario: A code that is empty after trimming becomes empty
    When a visitor registers a new username with referral code "   "
    Then Register succeeds
    And the UserRegistered event for the new user carries an empty referral code

  Scenario: A non-ASCII character is dropped
    When a visitor registers a new username with referral code "ABé12"
    Then Register succeeds
    And the UserRegistered event for the new user carries an empty referral code

  Scenario: Case is preserved
    When a visitor registers a new username with referral code "REFa1B2c3D4"
    Then the UserRegistered event for the new user carries referral code "REFa1B2c3D4"

  Scenario: A client-supplied user id or role escalation is ignored
    When a visitor registers with role "admin" and a client-supplied user id
    Then Register succeeds
    And the created account is a buyer with a server-generated user id
    And the UserRegistered event user id equals the generated id

  Scenario: Boundary length of 32 versus 33 characters
    When one visitor registers with a valid code of 32 characters and another with a valid-charset code of 33 characters
    Then both Register calls succeed
    And the first UserRegistered event carries the 32-character code
    And the second UserRegistered event carries an empty referral code

  Scenario: An unknown code never reveals itself
    When a visitor registers with a well-formed code that belongs to no user
    And another visitor registers with no code
    Then both Register responses have the same status code and body shape

  Scenario: One event per successful registration
    When a visitor registers a new username with referral code "ONE1"
    Then exactly one UserRegistered event for the new user is published
    And its registered_at is within 5 seconds of the account's created_at

  Scenario: A duplicate username writes no event
    Given a visitor registered a username with referral code "DUP1"
    When another visitor registers the same username with referral code "DUP2"
    Then Register returns the conflict error
    And still exactly one UserRegistered event exists for the original user
    And no UserRegistered event carries referral code "DUP2"

  Scenario: Gateway forwards the field
    When a visitor registers a new username with referral code "AB12cd"
    Then the UserRegistered event for the new user carries referral code "AB12cd"

  Scenario: Register stays public at the edge
    When an anonymous client calls Register at the gateway with a referral code
    Then the gateway answers with the Register result and not 401 or 403

  @destructive
  Scenario: Register does not depend on downstream services
    Given team-referral, team-promotion and Kafka are stopped
    When a visitor registers a new username with referral code "DOWN1"
    Then Register succeeds within the existing Register latency budget
    When Kafka is started again
    Then the UserRegistered event for the new user carries referral code "DOWN1"

  Scenario: Register logs contain no raw identifiers
    When a visitor registers a new username with a unique referral code
    And the identity and gateway logs are searched
    Then neither the new user id nor the referral code appears in any log line

  Scenario: A failed Register logs no raw identifiers either
    Given a visitor registered a username with referral code "LOGD1"
    When another visitor registers the same username with a unique referral code
    And the identity and gateway logs are searched
    Then neither the original user id nor the referral code appears in any log line

  Scenario: Register latency stays within budget with a referral code
    When 50 visitors register with a well-formed referral code
    Then the p95 Register latency is at most 10 ms above the recorded baseline
