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
