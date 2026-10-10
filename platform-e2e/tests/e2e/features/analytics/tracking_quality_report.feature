@tracking @analytics @admin
Feature: Admins can read a tracking data quality report
  AnalyticsQueryService/GetTrackingQualityReport summarises the tracking stream over a window:
  per-type counts, ingest lag, freshness, the sink counters and a status. It is admin-only at
  the edge (analytics-data-quality).

  Scenario: A re-sent event is counted as a skipped duplicate
    Given the seeded admin has read the tracking quality report for the last hour
    When a visitor posts the same view (same eventId and anonymousId) twice and the admin then reads the tracking quality report for the last hour
    Then the report's duplicates_skipped is at least 1 higher than it was before the two posts

  Scenario: Fresh views appear in the report
    Given the seeded admin
    When a visitor posts three views of a listing and the admin reads the report for the last hour
    Then the view count is at least 3, the latest ingest time is within the last 5 minutes, and the status is not degraded for stale

  Scenario: Views without a listing make the report incomplete
    Given the seeded admin
    When a visitor posts 50 views without a listingId and the admin reads the report for the last hour
    Then the view type's missing-listing ratio is above 0 and, if it exceeds the configured maximum, the status is DEGRADED with reason incomplete

  Scenario: A window out of range is rejected
    Given the seeded admin
    When the admin reads the report with a window of 500 hours
    Then the call fails with invalid_argument

  Scenario: Only admins can read the report
    Given a logged-in buyer
    When an anonymous client and then a logged-in buyer call GetTrackingQualityReport through the gateway
    Then the gateway answers HTTP 401 and then HTTP 403
