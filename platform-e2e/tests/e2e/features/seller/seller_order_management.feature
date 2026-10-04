@seller @order
Feature: Seller Order Management
  As a seller
  I want to view and manage all orders placed on my store
  So that I can fulfill shipments and prepare packing slips

  @needsSeller
  Scenario: Seller views order management list
    Given a seeded seller is logged in
    When the seller navigates to the seller orders page
    Then the seller orders dashboard displays the order status tabs

  @needsSeller
  Scenario: Order status tab is a link and survives reload
    Given a seeded seller is logged in
    When the seller navigates to the seller orders page
    And the seller selects the "Đang giao" order tab
    Then the URL contains "status=shipped"
    When the seller reloads the page
    Then the "Đang giao" order tab is still selected

  @needsSeller
  Scenario: Invalid order list parameters fall back to defaults
    Given a seeded seller is logged in
    When the seller opens the orders page with an invalid page and status
    Then the orders page renders its first page without an error
