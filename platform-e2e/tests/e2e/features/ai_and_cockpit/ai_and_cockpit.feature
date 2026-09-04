@ai
Feature: AI Shopping Assistant and Admin Observability Cockpit
  As an administrator and user
  I want to query the AI shopping assistant
  And monitor microservices health in the Admin Cockpit

  @smoke
  Scenario: Inspect system health on the Admin Cockpit
    When I navigate to the "admin cockpit" page
    Then I should see the observability telemetry HUD
