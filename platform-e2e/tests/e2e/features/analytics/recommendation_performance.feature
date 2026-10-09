@recommendations @analytics @admin
Feature: Admins can read recommendation performance per placement and model
  AnalyticsQueryService/GetRecommendationPerformance reports impressions, clicks and attributed
  conversions per (placement, model version) plus the fallback share per placement. It is
  admin-only at the edge (recsys-online-evaluation). Each scenario posts beacons under its own
  test-only placement id and model version so its rows are isolated from other tests.

  Scenario: Impressions and clicks are counted per placement and model
    Given the seeded admin for recommendation performance
    When a visitor posts 2 impressions of a recommendation row and 1 click on one of its listings with the same impressionId
    Then the admin's report for the last hour has a row for that placement and model with at least 1 impression, at least 2 item impressions and at least 1 click

  Scenario: A purchase after a recommended click is attributed
    Given the seeded admin for recommendation performance
    When a logged-in buyer clicks a recommended listing and then purchases that listing
    Then the report row for that placement and model counts that purchase

  Scenario: A purchase without a recommended click is not attributed
    Given the seeded admin for recommendation performance
    When a logged-in buyer purchases a listing they never clicked from a recommendation row
    Then no report row's purchase count includes that purchase

  Scenario: The fallback share is reported
    Given the seeded admin for recommendation performance
    When a visitor posts impressions for a placement with model version "serving-fallback" and with a real model version, 1 each
    Then the report gives that placement a fallback share above 0 and below 1

  Scenario: Only admins read recommendation performance
    Given a logged-in buyer for recommendation performance
    When an anonymous client and then a logged-in buyer call GetRecommendationPerformance through the gateway
    Then the gateway answers HTTP 401 and then HTTP 403 for recommendation performance
