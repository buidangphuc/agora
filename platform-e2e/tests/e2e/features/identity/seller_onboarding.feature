@auth @seller
Feature: Seller onboarding
  A signed-in buyer accepts the current seller terms through BecomeSeller and is a seller at once.

  Scenario: Seller onboarding: a signed-in buyer becomes a seller through the gateway
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version
    Then the call succeeds with a token carrying listing.write
    And creating a listing with that token no longer fails permission denied

  Scenario: Seller onboarding: a buyer becomes a seller
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version
    Then the new token keeps the session id of the caller and carries listing.write
    And the buyer's roles are buyer and seller

  Scenario: Seller onboarding: acceptance is recorded
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version
    Then the user row holds the current terms version and an acceptance time within 5 seconds of the call

  Scenario: Seller onboarding: the refresh token is unchanged
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version
    And the buyer refreshes the session with the refresh token issued at login
    Then the BecomeSeller response carries no refresh token
    And the refreshed token has the same session id and listing.write

  Scenario: Seller onboarding: an unknown terms version is refused
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with an empty terms version
    And the buyer calls BecomeSeller with the terms version "1999-01-01"
    Then both calls fail with invalid_argument
    And the buyer's roles are buyer only
    And no UserRoleGranted event is published for the buyer
    And the buyer can still become a seller with the current terms version

  Scenario: Seller onboarding: the terms version must match exactly
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version padded with spaces
    And the buyer calls BecomeSeller with the current terms version followed by "x"
    Then both calls fail with invalid_argument
    And the buyer's roles are buyer only
    And no UserRoleGranted event is published for the buyer
    And the buyer can still become a seller with the current terms version

  Scenario: Seller onboarding: a non-ACTIVE account is refused
    Given a registered buyer who cannot create listings
    And an admin restricts the buyer
    When the buyer calls BecomeSeller with the current terms version
    Then the call fails with permission_denied
    And the buyer's roles are buyer only
    And no UserRoleGranted event is published for the buyer
    And another active buyer can become a seller with the current terms version

  Scenario: Seller onboarding: an existing seller who is no longer ACTIVE gets no token
    Given a registered buyer who has become a seller
    And an admin restricts the buyer
    When the buyer calls BecomeSeller with the current terms version
    Then the call fails with permission_denied
    And the response carries no token

  Scenario: Seller onboarding: a revoked session cannot mint a token
    Given a registered buyer who cannot create listings
    And the buyer's first session is revoked from a second session
    When the buyer calls BecomeSeller with the revoked session's token
    Then the call fails with unauthenticated
    And the buyer's roles are buyer only
    And no UserRoleGranted event is published for the buyer
    And the buyer can still become a seller with the current terms version

  Scenario: Seller onboarding: an anonymous caller is refused at the edge
    Given a registered buyer who cannot create listings
    When an anonymous caller calls BecomeSeller with the current terms version
    Then the response status is 401
    And the buyer's roles are buyer only
    And the buyer can still become a seller with the current terms version

  Scenario: Seller onboarding: the first grant emits one event
    Given a registered buyer who cannot create listings
    When the buyer calls BecomeSeller with the current terms version
    Then exactly one UserRoleGranted event with role seller reaches identity.events within 10 seconds

  Scenario: Seller onboarding: a second call is idempotent
    Given a registered buyer who has become a seller
    When the buyer calls BecomeSeller with the current terms version again
    Then the call succeeds with a new token on the same session id
    And the stored terms version and acceptance time equal the first call's values
    And exactly one UserRoleGranted event with role seller was published for the buyer

  Scenario: Seller onboarding: concurrent calls emit one event
    Given a registered buyer who cannot create listings
    When the buyer sends two BecomeSeller calls concurrently
    Then both calls succeed
    And exactly one UserRoleGranted event with role seller reaches identity.events within 10 seconds

  Scenario: Seller onboarding: an admin restriction after onboarding still applies
    Given a registered buyer who has become a seller
    When an admin restricts the buyer
    And the buyer refreshes the session with the refresh token issued at login
    Then the refreshed token has no listing.write and carries status restricted

  Scenario: Seller onboarding: latency over 50 calls
    When 50 distinct buyers each call BecomeSeller once with the current terms version
    Then every call succeeds and the 95th percentile is below 200 ms

  Scenario: Seller onboarding: a rejected call is also fast
    When 50 distinct buyers each call BecomeSeller once with an unknown terms version
    Then every call fails with invalid_argument and the 95th percentile is below 200 ms
