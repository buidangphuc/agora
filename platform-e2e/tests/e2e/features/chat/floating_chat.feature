@chat
Feature: Floating Chat Bubble
  As a buyer or visitor
  I want to access the floating chat bubble from any page
  So that I can quickly reach customer support and shop inquiries

  Scenario: Visitor navigates to chat via floating bubble
    Given a buyer is logged in
    When the user opens the home page
    Then the floating chat bubble is visible on the bottom right
    When the user clicks the floating chat button
    Then the chat messenger page is displayed
