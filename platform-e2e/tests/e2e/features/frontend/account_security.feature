@buyer
Feature: Account security
  As a logged-in buyer
  I want an account security page listing my sessions and login history
  So that I can review and revoke access to my account

  Scenario: A logged-in buyer opens Account Security from the nav
    Given a logged-in buyer
    When the buyer opens Account Security from the "Bảo Mật" nav link
    Then the security page shows the sessions and login history sections

  Scenario: Revoking a session asks for confirmation first
    Given a logged-in buyer
    When the buyer opens the account security page directly
    And the buyer asks to revoke a session
    Then a confirmation dialog asks to revoke the session
    When the buyer dismisses the dialog with Escape
    Then the dialog is closed and the session can still be revoked

  Scenario: An anonymous visitor is redirected to login
    When an anonymous visitor opens the account security page
    Then they are redirected to the login page
