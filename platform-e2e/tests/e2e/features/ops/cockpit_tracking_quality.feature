@admin @tracking
Feature: The cockpit shows tracking data quality
  /admin/cockpit renders a "Tracking data quality" panel from the snapshot's tracking_quality
  section, and says the data is unavailable when team-analytics cannot be reached
  (analytics-data-quality).

  Scenario: The admin sees the tracking quality panel
    Given an admin is logged in
    And views were tracked and the tracking quality report counts them
    When the admin opens the cockpit tracking quality panel
    Then the "Tracking data quality" panel shows a status and a view count of at least 1

  # Stops team-analytics through the stack's compose wrapper (DC_WRAPPER); teardown starts it
  # again and waits until it is healthy and the report answers through the gateway.
  @destructive
  Scenario: The panel says unavailable when analytics is down
    Given an admin is logged in
    And team-analytics is stopped
    When the admin opens the cockpit tracking quality panel
    Then the "Tracking data quality" panel says the data is unavailable and shows no counts
