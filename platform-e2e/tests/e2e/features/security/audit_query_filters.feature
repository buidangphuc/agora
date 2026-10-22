Feature: Audit log query filters
  An admin can narrow QueryAuditLog by target id, action and time range through the
  gateway. Scenario names echo the spec scenarios of openspec/changes/archive/2026-10-18-audit-query-filters.
  Rows are seeded straight into audit_db with a fresh target per scenario.

  Scenario: An admin filters the audit log by target (Target id and target type narrow to one target)
    Given the seeded admin
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When the admin queries the audit log for target "A"
    Then exactly the 2 events of target "A" are returned, newest first
    And the page total is 2

  Scenario: The audit helper finds seeded events through the gateway (Helper finds seeded events through the gateway)
    Given the seeded admin
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When the helper lists the events of target "A"
    Then exactly the 2 events of target "A" are returned, newest first

  Scenario: An admin filters the audit log by action (Action filter is an exact match)
    Given the seeded admin
    And a fresh target "A" with events of actions "kyc.review", "kyc.reviewed" and "review.hide"
    When the admin queries the audit log for target "A" with action "kyc.review"
    Then only the events with action "kyc.review" are returned
    And the page total is 1

  Scenario: Filters combine with AND
    Given the seeded admin
    And a fresh target "A" with events for actor and action combinations
    When the admin queries target "A" with actor "adm-1" and action "order.force_fail"
    Then only the event matching actor "adm-1" and action "order.force_fail" is returned

  Scenario: The time range includes created_after and excludes created_before (Time range includes the lower bound and excludes the upper bound)
    Given the seeded admin
    And a fresh target "A" with events at 10:00:00, 11:00:00 and 12:00:00
    When the admin queries target "A" created after 10:00:00 and before 12:00:00
    Then the events at 11:00:00 and 10:00:00 are returned in that order
    And the page total is 2

  Scenario: A filter with no match returns an empty list (No match returns an empty page)
    Given the seeded admin
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When the admin queries the audit log for target id "e2e-aud-no-such-target"
    Then the call succeeds with no events, total 0 and no next cursor

  Scenario: A filtered result is paged newest first (Paging walks a filtered result)
    Given the seeded admin
    And a fresh target "A" with 25 events
    When the admin queries target "A" with page size 20 and then follows the next cursor
    Then page 1 holds 20 events with total 25 and a next cursor
    And page 2 holds the 5 oldest events and no next cursor
    And the events are newest first throughout

  Scenario: An inverted time range is refused (created_before earlier than created_after)
    Given the seeded admin
    And a fresh target "A" with events at 10:00:00, 11:00:00 and 12:00:00
    When the admin queries target "A" created after 12:00:00 and before 10:00:00
    Then the gateway answers HTTP 400 with code invalid_argument

  Scenario: A quote in a filter value is matched literally (Filter values are bound parameters)
    Given the seeded admin
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When the admin queries the audit log for target "A" with action "x' OR '1'='1"
    Then the call succeeds with no events, total 0 and no next cursor

  Scenario: Only an admin can query with filters (Non-admin cannot query with filters)
    Given a logged-in seller
    When the seller queries the audit log with target type "order" and target id "o-1"
    Then the gateway answers HTTP 403 and no event is returned

  Scenario: Only an admin can query with filters (Anonymous caller cannot query with filters)
    Given an anonymous client
    When the anonymous client queries the audit log with action "order.force_fail"
    Then the gateway answers HTTP 401 and no event is returned

  Scenario: Only an admin can query with filters (Helper query with a non-admin token is refused)
    Given a logged-in seller
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When the helper queries target "A" with the seller token
    Then the gateway answers HTTP 403 and no event is returned

  @destructive
  Scenario: The audit helper restarts team-audit after a stop (Helper restarts team-audit after a stop)
    Given the seeded admin
    When team-audit is stopped through the helper
    And the scenario teardown runs
    Then a later admin QueryAuditLog through the gateway returns HTTP 200
    When team-audit is stopped through the helper
    And team-audit is started again through the helper
    Then a later admin QueryAuditLog through the gateway returns HTTP 200
    And stopping team-audit twice through the helper is harmless

  @destructive
  Scenario: The audit helper fails clearly while team-audit is down (Query fails clearly while team-audit is down)
    Given the seeded admin
    And a fresh target "A" with 2 events and a fresh target "B" with 1 event
    When team-audit is stopped through the helper
    Then the helper query raises AuditUnavailable instead of returning an empty list
    And the helper events_for raises AuditUnavailable instead of returning an empty list
