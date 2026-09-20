@seller @smoke @analytics @fintech
Feature: Seller Cockpit, Analytics Funnel and Demand Forecast Journey
  As a seller on Agora marketplace, I want to publish and update listings, analyze the full conversion
  funnel from impressions down to purchases, and retrieve probabilistic demand and restock forecasts
  with multi-quantile uncertainty bounds and safety stock recommendations.

  @needsSeller
  Scenario: Seller publishes listing, monitors conversion funnel analytics, and queries demand forecast
    Given a seller is logged in
    When the seller publishes a new listing with inventory stock
    Then the listing is published and visible in seller listings
    When the seller updates the listing price and inventory stock
    Then the updated listing details are saved successfully
    When the seller queries the conversion funnel analytics
    Then the conversion funnel reports impressions, views, adds, checkouts, and orders
    When the seller queries probabilistic demand forecast for the listing
    Then the forecast returns 14-day P10, P50, and P90 quantile distributions with safety stock and reorder point
