@buyer @recommendations @tracking @destructive
Feature: Storefront beacons carry the server request id (recs-serving-safeguards)
  The recommendation row sends the response's request_id as impressionId and its placement_id as
  placementId, so impressions can be joined to what the server served.

  # Destructive: the buyer's recommendation list is seeded in the serving Redis (saved and restored
  # in teardown) so the homepage shows a row for the scenario's own buyer.

  @needsBuyer @needsListing
  Scenario: Storefront beacons carry the server request id
    Given a buyer is logged in
    When a buyer opens the homepage and its recommendation row is shown
    Then the impression beacons of that row carry an impressionId equal to the request_id the storefront received, and placementId "home_feed"
