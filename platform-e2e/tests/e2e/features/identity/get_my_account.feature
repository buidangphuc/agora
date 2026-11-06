@auth
Feature: GetMyAccount returns the caller's own account
  AuthService.GetMyAccount takes no request fields and answers from the verified Principal only.

  Scenario: A signed-in user reads their own account
    Given a registered buyer
    When the buyer calls GetMyAccount through the gateway
    Then the response user id is that buyer's id
    And created_at is within 5 seconds of the registration time

  Scenario: An anonymous call is refused
    When a caller with no token calls GetMyAccount at the gateway
    Then the gateway answers 401

  Scenario: A caller can never read another user's account
    Given a registered buyer
    And a second registered buyer
    When the second buyer calls GetMyAccount naming the first buyer's id in a header and in the body
    Then the response user id is the second buyer's own id
    And nothing of the first buyer's account is returned

  Scenario: A seller and a buyer are both allowed
    Given a registered buyer
    And a registered seller
    When the seller and the buyer each call GetMyAccount through the gateway
    Then each response carries that caller's own user id

  Scenario: A principal whose account no longer exists
    Given a registered buyer
    And the buyer's user row is removed
    When the buyer calls GetMyAccount through the gateway
    Then the call fails with not_found and returns no account data
