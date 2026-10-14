@auth @buyer
Feature: Frontend session continuity
  As a signed-in shopper
  I want the storefront to renew my session silently and end it for real on logout
  So that I stay signed in for up to 30 days without seeing a login form

  # S1
  Scenario: Login sets the session and refresh cookies
    Given a registered buyer
    When the buyer signs in through the login form
    Then the browser holds a session cookie lasting 3600 seconds
    And the browser holds an agora_refresh cookie that is httpOnly, lax, path "/" and lasts 2592000 seconds
    And the agora_refresh cookie is secure exactly when the session cookie is secure

  # S2
  Scenario: Register sets the session and refresh cookies
    When a new buyer signs up through the register form
    Then the browser holds a session cookie lasting 3600 seconds
    And the browser holds an agora_refresh cookie that is httpOnly, lax, path "/" and lasts 2592000 seconds
    And the agora_refresh cookie is secure exactly when the session cookie is secure

  # S4
  Scenario: Failed login sets no cookie
    Given a registered buyer
    When the buyer signs in through the login form with a wrong password
    Then a login error is displayed
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie

  # S5, S21
  Scenario: Expired access token with a valid refresh cookie is renewed on the same request
    Given a buyer with a live gateway session
    And the browser holds a session cookie that expired a minute ago and the refresh cookie
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the browser session cookie is a usable token of the same session
    And the agora_refresh cookie was rotated and still works
    And the agora_refresh cookie keeps the attributes of a login

  # S6
  Scenario: Missing access token with a valid refresh cookie is renewed
    Given a buyer with a live gateway session
    And the browser holds only the refresh cookie
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the browser session cookie is a usable token of the same session
    And the agora_refresh cookie was rotated and still works

  # S20
  Scenario: Access token expiring exactly now is treated as unusable
    Given a buyer with a live gateway session
    And the browser holds a session cookie expiring exactly now and the refresh cookie
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the agora_refresh cookie was rotated and still works

  # S7
  Scenario: Usable access token causes no refresh
    Given a buyer with a live gateway session
    And the browser holds the real session cookie and the refresh cookie
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the session cookie is unchanged
    And the agora_refresh cookie is unchanged

  # S8
  Scenario: Expired access token without a refresh cookie stays anonymous
    Given a buyer with a live gateway session
    And the browser holds a session cookie that expired a minute ago and no refresh cookie
    When the visitor loads the home page
    Then the page renders anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie

  # S9
  Scenario: Revoked refresh cookie clears both cookies
    Given a buyer with a live gateway session
    And the buyer's session has been revoked
    And the browser holds a session cookie that expired a minute ago and the refresh cookie
    When the visitor loads the home page
    Then the page renders anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie

  # S9
  Scenario: Garbage refresh cookie clears both cookies
    Given the browser holds a garbage refresh cookie and no session cookie
    When the visitor loads the home page
    Then the page renders anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie

  # S19
  Scenario: Grace replay of the refresh cookie signs in and keeps the cookie
    Given a buyer with a live gateway session
    And the session already rotated its refresh token once
    And the browser holds the first refresh token and no session cookie
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the browser session cookie is a usable token of the same session
    And the agora_refresh cookie is unchanged

  # S18
  Scenario: Parallel requests sharing one refresh cookie both render signed in
    Given a buyer with a live gateway session
    When 2 requests for the home page carry the same refresh cookie and no session cookie at once
    Then every response renders signed in
    And the buyer's session is still active
    And a refresh token handed back to the browser still works

  # S13
  Scenario: Logout revokes the server-side session and clears both cookies
    Given a buyer with a live gateway session
    And the browser holds the real session cookie and the refresh cookie
    And the visitor loads the home page
    When the shopper clicks logout
    Then the shopper lands on the home page anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie
    And the gateway rejects the old access token
    And the gateway rejects the refresh token

  # S16
  Scenario: A revoked session cannot refresh through the browser
    Given a buyer with a live gateway session
    And the browser holds the real session cookie and the refresh cookie
    And the visitor loads the home page
    And the shopper clicks logout
    When the old cookies are put back in the browser
    And the visitor loads the home page
    Then the page renders anonymous
    And the browser holds no agora_refresh cookie

  # S15
  Scenario: Logout with only the refresh cookie still revokes the session
    Given a buyer with a live gateway session
    And the browser holds only the refresh cookie
    And the visitor loads the home page
    And the session cookie is removed from the browser
    When the shopper clicks logout
    Then the shopper lands on the home page anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie
    And the gateway rejects the old access token
    And the gateway rejects the refresh token

  # S10
  @destructive
  Scenario: Identity outage leaves the cookies untouched
    Given a buyer with a live gateway session
    And the browser holds a session cookie that expired a minute ago and the refresh cookie
    And team-identity is stopped
    When the visitor loads the home page
    Then the page renders anonymous
    And the session cookie is unchanged
    And the agora_refresh cookie is unchanged
    When team-identity is started again
    Then the original refresh token still refreshes

  # S14
  @destructive
  Scenario: Revoke outage still logs the shopper out locally
    Given a buyer with a live gateway session
    And the browser holds the real session cookie and the refresh cookie
    And the visitor loads the home page
    And team-identity is stopped
    When the shopper clicks logout
    Then the shopper lands on the home page anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie
    When team-identity is started again
    Then the browser is still anonymous

  # S24
  @destructive
  Scenario: Logout refresh-only path failing still clears both cookies
    Given a buyer with a live gateway session
    And the browser holds only the refresh cookie
    And the visitor loads the home page
    And the session cookie is removed from the browser
    And team-identity is stopped
    When the shopper clicks logout
    Then the shopper lands on the home page anonymous
    And the browser holds no session cookie
    And the browser holds no agora_refresh cookie
    When team-identity is started again
    Then the browser is still anonymous
