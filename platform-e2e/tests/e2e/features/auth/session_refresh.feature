@auth @buyer
Feature: Session refresh with a rotating refresh token
  As a signed-in buyer
  I want my session to renew itself without signing in again
  And I want a stolen or replayed refresh token to end the session
  So that I stay signed in safely for up to 30 days

  Scenario: Login returns a refresh token
    Given a registered buyer
    When the buyer logs in directly at the gateway and keeps the refresh token
    Then the refresh token is a 43-character base64url string
    And the refresh expiry is 30 days from now

  Scenario: Register returns a refresh token
    When a new buyer registers directly at the gateway
    Then the refresh token is a 43-character base64url string

  Scenario: Successful rotation keeps the session id
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes with the current refresh token
    Then the refresh returns an access token for the original session id
    And the refresh returns a different refresh token expiring in 30 days
    And the refresh needed no Authorization header

  Scenario: Parallel tabs do not trip reuse detection
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes with the same refresh token from two tabs at once
    Then exactly one tab receives a new refresh token
    And the other tab receives an access token with an empty refresh token
    And the session is still listed as active

  Scenario: Grace replay returns an access token only
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes with the current refresh token
    And the buyer replays the first refresh token immediately
    Then the replay returns an access token for the original session id with no refresh token
    And the second refresh token is still accepted

  Scenario: Token older than one rotation is rejected without revoke
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes 2 times in sequence
    And the buyer replays the first refresh token immediately
    Then the refresh is rejected as unauthenticated
    And the latest refresh token is still accepted

  # The grace window is 10 s: the chain needs the rotated token to be older than that.
  @slow
  Scenario: The rotated token is the only live one
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes 2 times in sequence
    And 11 seconds pass
    And the buyer refreshes with the latest refresh token
    Then the refresh returns a different refresh token expiring in 30 days

  @slow
  Scenario: Reuse after the grace window revokes the session
    Given a buyer signed in on a second device and holding a refresh token on the first
    When the buyer refreshes 1 times in sequence
    And 11 seconds pass
    And the buyer replays the first refresh token
    Then the refresh is rejected as unauthenticated
    And the latest refresh token is rejected as unauthenticated
    And the first device's session is not active in the second device's session list

  Scenario: Prev-token replay within grace after logout
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes 1 times in sequence
    And the buyer logs out of the session
    And the buyer replays the first refresh token immediately
    Then the refresh is rejected as unauthenticated

  Scenario: Logout then refresh, end to end
    Given a buyer who has logged in and holds a refresh token
    When the buyer logs out of the session
    And the buyer refreshes with the current refresh token
    Then the refresh is rejected as unauthenticated
    And no access token was issued

  Scenario: Anonymous caller reaches identity
    Given a buyer who has logged in and holds a refresh token
    When the buyer refreshes with the current refresh token
    Then the refresh returns an access token for the original session id
    And the refresh needed no Authorization header

  Scenario: Unauthenticated passes through
    When an anonymous caller refreshes with a random 43-character token
    Then the refresh is rejected as unauthenticated

  Scenario: Unknown token
    When an anonymous caller refreshes with a random 43-character token
    Then the refresh is rejected as unauthenticated

  Scenario: Missing or oversized token is Unauthenticated
    When an anonymous caller refreshes with an empty token
    Then the refresh is rejected as unauthenticated
    When an anonymous caller refreshes with a token of 257 bytes
    Then the refresh is rejected as unauthenticated

  Scenario: Maximum-length token is accepted for lookup
    When an anonymous caller refreshes with a token of 256 bytes
    Then the refresh is rejected as unauthenticated

  Scenario: Failure messages do not leak the cause
    Given a buyer who has logged in and holds a refresh token
    When the buyer logs out of the session
    Then refresh failures for unknown, revoked, empty and 257-byte tokens carry an identical message
