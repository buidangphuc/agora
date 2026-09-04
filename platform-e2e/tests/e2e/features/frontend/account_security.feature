@buyer
Feature: Account security
  As a logged-in buyer
  I want an account security page listing my sessions and login history
  So that I can review and revoke access to my account

  Scenario: A logged-in buyer opens Account Security from the nav
    Given a logged-in buyer
    When the buyer opens Account Security from the "Bảo Mật" nav link
    Then the security page shows the sessions and login history sections

  Scenario: An anonymous visitor is redirected to login
    When an anonymous visitor opens the account security page
    Then they are redirected to the login page
