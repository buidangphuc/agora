@auth @buyer
Feature: Role tokens and login history
  As a user
  I want my token to carry the right scopes and my login attempts recorded
  So that AI and recommendations work for me and I can see suspicious attempts

  Scenario: A buyer token carries the AI scopes and no service scope
    Given a registered buyer account
    When the buyer logs in through the gateway
    Then the token scopes include recommendations:read and ai:use
    And the token scopes include none of the service-only scopes

  Scenario: Login history shows a failed and a successful attempt
    Given a registered buyer account
    When the buyer logs in once with a wrong password and then with the right one
    And the buyer lists their login history through the gateway
    Then the history contains a failure entry followed by a success entry
