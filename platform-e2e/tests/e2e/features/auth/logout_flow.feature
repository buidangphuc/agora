@auth
Feature: User Logout Flow
  As an authenticated user
  I want to log out of my session from the global header
  So that my account is securely disconnected from the device

  Scenario: Authenticated user logs out via header
    Given a buyer is logged in
    When the user opens the home page
    And the user clicks the logout button
    Then the header displays the login and register links
