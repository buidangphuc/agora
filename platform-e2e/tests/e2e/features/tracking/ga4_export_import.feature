@tracking @analytics
Feature: GA4 export import, attributed channel funnel and reconciliation
  The daily ga4import job reads a GA4 export day (Parquet fixture with the GA4 export schema,
  GA4_EXPORT_SOURCE=fixture) into ga4_sessions, first-party events are attributed to those
  sessions by GA client id, ops read the funnel split by channel (admin only), and daily
  GA4 vs first-party counts are reconciled in the tracking quality report.

  Scenario: A missing import is reported, not treated as divergence
    Given the seeded admin
    And no import run exists for yesterday
    When the admin reads the tracking quality report for the last hour
    Then the report carries reason ga4_missing and no ga4_divergence reason

  Scenario: A missing import adds a reason only
    Given the seeded admin
    And no import run exists for yesterday
    When the admin reads the tracking quality report for the last hour
    Then the report contains reason ga4_missing
    And the report status is not degraded because of ga4

  Scenario: Import of a fixture day produces sessions
    Given a GA4 fixture day with 3 sessions of 2 distinct users
    When the ga4import job runs for the fixture date
    Then ga4_sessions holds exactly 3 rows for that date
    And every one of those rows has a source, a medium and a session id

  Scenario: Re-running the same date is idempotent
    Given a GA4 fixture day with 3 sessions of 2 distinct users
    When the ga4import job runs for the fixture date
    And the ga4_sessions rows of the fixture date are noted
    And the ga4import job runs for the fixture date
    Then the ga4_sessions rows of the fixture date equal the noted rows

  Scenario: ga4_sessions carries export_date
    Given a GA4 fixture day with 3 sessions of 2 distinct users
    And a second GA4 fixture day with 2 sessions of 1 user
    When the ga4import job runs for the first and then the second fixture date
    And the ga4_sessions rows of the second fixture date are noted
    And the ga4import job runs for the first fixture date
    Then every ga4_sessions row of both dates has a non-null export_date
    And the ga4_sessions rows of the second fixture date equal the noted rows

  Scenario: ga4_sessions stores ended_at
    Given a GA4 fixture session with events at 10:00, 10:07 and 10:15
    When the ga4import job runs for the fixture date
    Then the session row has ts 10:00 and ended_at 10:15

  Scenario: Landing page query strings are removed
    Given a GA4 fixture session whose page_location is "https://agora.test/p/1?email=a@b.com&utm_source=x"
    When the ga4import job runs for the fixture date
    Then the stored landing_page equals "https://agora.test/p/1"

  Scenario: Empty day is a valid import
    Given a GA4 fixture day with zero events
    When the ga4import job runs for the fixture date
    Then the job succeeds
    And ga4_sessions holds exactly 0 rows for that date
    And an import run with rows 0 is recorded for that date

  Scenario: A missing export table fails without changing data
    Given a GA4 fixture day with 3 sessions of 2 distinct users
    And the ga4import job has run for the fixture date
    When the ga4import job runs for a date whose export table does not exist
    Then the job exits with an error naming that date
    And the ga4_sessions rows of the fixture date are unchanged

  Scenario: Invalid threshold is rejected at startup
    When the ga4import job starts with GA4_RECONCILE_MAX_DIFF_RATIO=-1
    Then the job exits with an error naming GA4_RECONCILE_MAX_DIFF_RATIO

  Scenario: Import of 5M rows meets the time budget
    Given a GA4 fixture day with 5000000 events
    When the ga4import job runs for the fixture date
    Then the job finished in 10 minutes or less
    And an import run is recorded for that date

  Scenario: Counts are split by channel
    Given a GA4 export for today with a "spring" session and a "summer" session for two visitors
    And the ga4import job has run for the fixture date
    When one visitor posts 4 views and the other visitor posts 2 views, each with their GA client id
    And the admin requests the channel funnel around now
    Then the funnel has a spring row with 4 views and a summer row with 2 views

  Scenario: A window with no events returns no rows
    Given an admin is logged in
    When the admin requests GetChannelFunnel over a range that holds no events
    Then the channel funnel call returns 200 with zero rows

  Scenario: An invalid window is rejected
    Given an admin is logged in
    When the admin requests GetChannelFunnel with from after to
    Then the channel funnel call returns 400

  Scenario: Only admins can read the channel funnel
    Given a logged-in buyer
    When an anonymous client and then a logged-in buyer request the channel funnel through the gateway
    Then the gateway answers HTTP 401 and then HTTP 403 for the channel funnel

  Scenario: Funnel query meets p95 latency
    Given the seeded admin
    When the admin calls GetChannelFunnel 100 times over a 30-day window
    Then every call succeeded
    And the p95 latency is under 2 seconds

  Scenario: Divergence beyond the threshold raises the alert
    Given the seeded admin
    And a visitor has posted 20 views today
    And a GA4 export for today whose view count is half of the first-party view count
    When the ga4import job runs for the fixture date
    And the admin reads the tracking quality report for the last hour
    Then the report status is DEGRADED with reason ga4_divergence
