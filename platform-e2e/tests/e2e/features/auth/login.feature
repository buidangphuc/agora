@auth
Feature: Authentication
  As a user, I can sign in so that I can access my account.

  @smoke
  Scenario: Valid login through the UI
    Given the login page is open
    When the user logs in as the seeded buyer
    Then the user lands on the home page

  Scenario: Invalid credentials are rejected
    Given the login page is open
    When the user logs in with username "khong_ton_tai" and password "saibet123"
    Then a login error is shown
