@analytics @admin @search
Feature: Analytics reports search performance per model version
  AnalyticsQueryService/GetSearchPerformance reports impressions, clicks and attributed purchases of
  placement search_results per model version, admin only. Each scenario posts beacons under its own
  fresh model version so its rows are isolated from other tests. Scenarios are named after the
  `#### Scenario:` titles of change search-attribution (spec search-attribution).

  Scenario: CTR per model version
    Given the seeded admin for search performance
    When a visitor posts 10 item impressions and 2 clicks under one model version and 10 item impressions and 1 click under another
    Then the admin's search performance report has a row with ctr 0.2 for the first and ctr 0.1 for the second

  Scenario: Only search results are counted
    Given the seeded admin for search performance
    When a visitor posts impressions under placement "similar_items" with a fresh model version and a search_results impression under another
    Then the admin's search performance report has no row for the similar_items model version

  Scenario: A click on a listing the search did not show is not credited
    Given the seeded admin for search performance
    When a visitor posts a search impression of listing A and a click on listing B with the same impression_id
    Then the search performance row of that model version has 1 item impression and 0 clicks

  Scenario: A reused impression id under another model is separate
    Given the seeded admin for search performance
    When a visitor posts impressions with one impression_id under two model versions
    Then the admin's search performance report has two rows, one per model version

  Scenario: Purchases come from server order facts
    Given the seeded admin for search performance
    When a logged-in buyer clicks a search result and posts a purchase tracking event for it but pays no order
    Then the search performance row of that model version has 1 click and 0 purchases

  Scenario: A zero window resolves to 24 hours
    Given the seeded admin for search performance
    When the admin calls GetSearchPerformance with window_hours 0
    Then the search performance response window_hours equals 24

  Scenario: window_hours out of range is rejected
    Given the seeded admin for search performance
    When the admin calls GetSearchPerformance with window_hours 1000
    Then the gateway answers HTTP 400 with code invalid_argument for search performance

  Scenario: A non-admin is denied
    Given a seller and a buyer for search performance
    When the seller and then the buyer call GetSearchPerformance through the gateway
    Then the gateway answers HTTP 403 for both

  Scenario: An unauthenticated caller is denied
    When an anonymous client calls GetSearchPerformance through the gateway
    Then the gateway answers HTTP 401 for search performance
