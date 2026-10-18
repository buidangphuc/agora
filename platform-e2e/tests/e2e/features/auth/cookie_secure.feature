@auth @buyer
Feature: Frontend auth cookies honour the proxy scheme
  As an operator running the storefront behind a TLS proxy
  I want the auth cookies to be Secure exactly when the request arrived over https
  So that local http login keeps working and deployed logins are never sent in clear

  # The e2e stack runs with COOKIE_SECURE unset by design.

  Scenario: Cookie secure: unset keeps the proxy rule
    Given a registered buyer
    And requests carry the x-forwarded-proto header "https"
    When the buyer signs in through the login form
    Then every Set-Cookie for session, agora_refresh and agora_auth_evt carries the Secure attribute

  Scenario: Cookie secure: unset and plain http (boundary)
    Given a registered buyer
    And the Set-Cookie headers are recorded
    When the buyer signs in through the login form
    Then every Set-Cookie for session, agora_refresh and agora_auth_evt omits the Secure attribute
    And the browser holds a session cookie lasting 3600 seconds
    When the browser is cleared and the buyer signs in again with the x-forwarded-proto header "https"
    Then every Set-Cookie for session, agora_refresh and agora_auth_evt carries the Secure attribute

  Scenario: Cookie secure: middleware refresh unset over https
    Given a buyer with a live gateway session
    And the browser holds a session cookie that expired a minute ago and the refresh cookie
    And the Set-Cookie headers are recorded
    When the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the response set session and agora_refresh without the Secure attribute
    When the session expires again and requests carry the x-forwarded-proto header "https"
    And the visitor loads the home page
    Then the page renders signed in without a second navigation
    And the response set session and agora_refresh with the Secure attribute
