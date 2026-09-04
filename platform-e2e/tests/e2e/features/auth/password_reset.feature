@auth
Feature: Password Management and Reset Token Lifecycle
  As a registered user
  I want to be able to request a password reset
  And securely change my password

  @smoke @needsBuyer
  Scenario: User requests a password reset token
    Given I am logged in as a buyer via API
    When I request a password reset token for my account
    Then a valid password reset token should be generated
