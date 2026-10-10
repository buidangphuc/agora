@buyer @destructive
Feature: Account failure paths with a real service outage (ui-phase-account)
  Each scenario genuinely stops an agora container (team-identity behind the sessions and the login
  history, team-notification behind /notifications) and restores it in teardown. Destructive:
  serial lane only.

  Scenario: Gateway read failure
    Given a uif buyer is logged in
    When team-identity is stopped to force a real failure
    And the uif buyer opens the security page
    Then the sessions section shows an inline error alert with a retry action and the login history section is rendered
    When team-identity is running again and answers through the gateway
    And the buyer presses the retry action of the sessions alert
    Then the sessions section lists the buyer's session

  Scenario: Route error offers recovery
    Given a uif buyer is logged in
    When team-notification is stopped to force a real failure
    And the uif buyer opens the notifications page
    Then the route error result offers a retry button
    When team-notification is running again and answers through the gateway
    And the buyer presses the retry button of the error
    Then the notifications page is rendered
