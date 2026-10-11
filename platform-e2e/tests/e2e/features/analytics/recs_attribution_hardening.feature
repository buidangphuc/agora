@recommendations @analytics @admin @integration
Feature: Recommendation attribution counts server truth and matches clicks to impressions
  GetRecommendationPerformance credits purchases from paid order lines (not purchase beacons), counts
  a click only on a listing its impression showed, keeps a reused impression id's placements and models
  apart, and leaves still-open attribution windows out of the conversion rate.
  (recs-attribution-hardening / recsys-online-evaluation)

  Each scenario posts beacons under its own test-only placement id and model version so its rows are
  isolated from other tests. The scenarios that need a purchase pay a real order through the gateway;
  the warehouse reads it from order facts once team-order's event has been consumed, so they poll.

  Scenario: A purchase after a recommended click is attributed
    When a logged-in buyer clicks a recommended listing and then pays an order for that listing
    Then the report row for that placement and model counts that purchase

  Scenario: A purchase beacon without an order is not counted
    When a logged-in buyer clicks a recommended listing and posts a purchase tracking event for it, but pays no order
    Then the report row for that placement and model counts the click and 0 purchases

  Scenario: A click on a listing the impression did not show is not counted
    When a visitor posts an impression of listing A and a click on listing B with the same impressionId
    Then the report row for that placement and model counts the impression and 0 clicks

  Scenario: A reused impression id keeps its placements and models apart
    When a visitor posts impressions of one listing with the same impressionId under two placements with two models, and one click carrying that impressionId, the first placement and the first model
    Then the report has a row for each placement and model with at least 1 impression, and only the first row counts the click

  Scenario: A conversion on a still-open attribution window is left out of the rate
    When a logged-in buyer clicks a recommended listing and pays an order for it within the attribution window, and the report is read right away
    Then the report row for that placement and model counts 1 click and 1 purchase, and its conversion rate is 0
