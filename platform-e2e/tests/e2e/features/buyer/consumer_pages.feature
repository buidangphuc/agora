@buyer
Feature: Consumer pages
  As a buyer, my account pages and search suggestions work.

  Scenario: Notification center renders
    Given a buyer is logged in
    When I navigate to the "notifications" page
    Then the notification center is displayed

  Scenario: Chat conversations render
    Given a buyer is logged in
    When I navigate to the "chat" page
    Then the chat page is displayed

  Scenario: Search suggests keywords
    Given the "home" page is open
    When the buyer types a partial search keyword
    Then a search suggestion appears
