@buyer @engagement @needsBuyer
Feature: Loyalty daily check-in
  As a logged-in buyer
  I want a daily check-in widget on the home page
  So that I can build a streak and earn coins

  Background:
    Given I am logged in as a buyer via API

  Scenario: The check-in widget renders on the home page
    When the buyer opens the home page
    Then the loyalty check-in widget shows the streak and coin balance

  Scenario: Checking in advances the streak and coin balance
    When the buyer opens the home page
    And the buyer checks in for the day
    Then the streak advances and coins are earned
