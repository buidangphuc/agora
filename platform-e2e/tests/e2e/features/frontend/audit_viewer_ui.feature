@admin
Feature: Admin audit viewer in the browser (frontend-audit-viewer)
  /admin/audit lists audit events newest first, 20 per page, with filters kept in the URL. Events
  are seeded straight into audit_db with explicit timestamps (audit_support.seed_events) under
  per-run unique targets and actors; every assertion is on the rendered page.

  Scenario: Seeded events paginate end to end - Admin pages through 25 events
    Given 25 audit events seeded one second apart for one fresh target
    And an admin is logged in
    When the admin opens the audit page filtered to that target
    Then the audit table shows 20 rows with the newest event first and a next page link
    When the admin follows the next page link
    Then the audit table shows the remaining 5 events and no next page link

  Scenario: Filtering by target narrows the list - Admin filters events by target and action
    Given events seeded for two different targets of the same type
    And an admin is logged in
    When the admin opens the audit page filtered to the first target
    Then every audit row shows the first target id and no row shows the second

  Scenario: Filtering by action and actor - Admin filters events by target and action
    Given events with the actions "kyc.review" and "order.force_fail" seeded for a unique actor and another actor
    And an admin is logged in
    When the admin opens the audit page filtered to that target
    And the admin submits the form with the action "kyc.review" and the seeded actor id
    Then only audit rows with the action "kyc.review" and the seeded actor are listed

  Scenario: Day edge in Vietnam time - Day edge follows Vietnam time
    Given events at 2026-10-05T16:59:59Z and 2026-10-05T17:00:00Z seeded for one fresh target
    And an admin is logged in
    When the admin opens the audit page filtered to that target
    And the admin submits the form with from "2026-10-05" and to "2026-10-05"
    Then the audit list shows the 16:59:59Z event and does not show the 17:00:00Z event

  Scenario: Date range excludes events outside it - Admin filters events by date range
    Given 1 audit event seeded on 2026-09-01 for one fresh target
    And an admin is logged in
    When the admin opens the audit page filtered to that target
    And the admin submits the form with from "2026-01-01" and to "2026-01-01"
    Then the audit page shows "Không có sự kiện nào" and no audit rows

  Scenario: End date before start date in the browser - Reversed date range shows an error
    Given an admin is logged in
    When the admin opens the audit page
    And the admin submits the form with from "2026-10-05" and to "2026-10-01"
    Then the audit page shows the invalid-range message and no audit table
    And the form keeps the dates "2026-10-05" and "2026-10-01"

  Scenario: Anonymous browser visit - An anonymous visitor is sent to login
    Given no one is logged in
    When the visitor opens the audit page
    Then the browser lands on the login page

  Scenario: A buyer opening the page in the browser is refused - A buyer cannot open the audit page
    Given I am logged in as a buyer via API
    When the visitor opens the audit page
    Then the audit page shows "Cần tài khoản Quản trị" and no audit table

  Scenario: Following the cockpit link - Admin reaches the audit page from the cockpit
    Given an admin is logged in
    When the admin opens the cockpit and clicks "Nhật ký kiểm toán"
    Then the browser is on the audit page with the heading "Nhật ký kiểm toán"
