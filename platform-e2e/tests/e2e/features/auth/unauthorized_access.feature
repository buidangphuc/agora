@auth
Feature: Unauthorized access and login validation
  As a secure marketplace, protected areas require authentication and invalid logins fail cleanly.

  Scenario: Unauthenticated visitor accessing seller dashboard is redirected to login
    Given the "seller listings" page is open
    Then the user is redirected to the login page

  Scenario: Unauthenticated visitor accessing orders page is redirected to login
    Given the "account orders" page is open
    Then the user is redirected to the login page
