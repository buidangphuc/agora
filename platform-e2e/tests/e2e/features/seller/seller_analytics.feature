@seller @needsSeller
Feature: Seller analytics and revenue dashboard
  As a shop owner, I can monitor revenue, total orders, and sales performance.

  Scenario: Seller views revenue analytics dashboard
    Given a seeded seller is logged in
    When the seller opens the analytics page
    Then the seller analytics dashboard is displayed
    And the revenue metrics summary is visible

  Scenario: Range tab updates the URL
    Given a seeded seller is logged in
    When the seller opens the analytics page
    And the seller selects the "30 ngày" range tab
    Then the URL contains "range=30d"
    And the "30 ngày" range tab is current
