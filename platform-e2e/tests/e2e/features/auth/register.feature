@auth
Feature: Registration
  As a visitor, I can create an account so that I can buy and sell.

  Scenario: Visitor registers a new buyer account
    Given the "register" page is open
    When the visitor registers a new buyer account
    Then the user lands on the home page
