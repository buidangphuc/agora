@engagement
Feature: Listing reports
  A signed-in user reports a listing with a reason code; an admin lists open reports and closes
  each as ACTIONED or DISMISSED with an audited note (engagement-listing-reports, initiative
  q6-trust-enforcement). Everything runs through the gateway. Scenario names echo the spec
  scenario titles so spec_sync can match them.

  Scenario: Listing reports: Report created end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    When "b1" reports "L1" with reason "scam"
    Then the report call succeeds with status OPEN reported by "b1"
    When "b1" reports "L1" with reason "scam"
    Then the report call returns the same report id as before
    And the admin sees exactly 1 report for "L1"

  Scenario: Listing reports: Anonymous report is refused end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    When an anonymous client reports "L1" with reason "scam"
    Then the report call is refused with HTTP 401
    When "b1" reports "L1" with reason "scam"
    Then the report call succeeds with status OPEN reported by "b1"
    And the admin sees exactly 1 report for "L1"

  Scenario: Listing reports: Seller self-report is refused end to end
    Given a seller with a published listing "L1"
    And the engagement service knows the seller of "L1"
    And a buyer "b1"
    And an admin
    When the seller reports "L1" with reason "scam"
    Then the report call fails with "failed_precondition"
    When "b1" reports "L1" with reason "scam"
    Then the report call succeeds with status OPEN reported by "b1"
    And the admin sees exactly 1 report for "L1"

  Scenario: Listing reports: Seller cannot read reports end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    When "b1" reports "L1" with reason "counterfeit" and note "hàng nhái secret-note"
    And the seller lists reports for "L1"
    Then the list call fails with "permission_denied"
    And the list response body contains neither the reporter id of "b1" nor the note
    And the admin sees exactly 1 report for "L1"

  Scenario: Listing reports: Admin works a report end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    And "b1" has reported "L1" with reason "scam"
    When the admin lists open reports for "L1"
    Then the list contains the report of "b1"
    When the admin closes that report as ACTIONED with the note "đã gỡ tin"
    Then the close call succeeds with status ACTIONED
    And the open reports for "L1" no longer contain the report
    And the audit log holds exactly 1 "report.resolve" event for the report by the admin with the note "đã gỡ tin"

  @destructive
  Scenario: Listing reports: Audit outage blocks closing end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    And "b1" has reported "L1" with reason "scam"
    And team-audit is stopped
    When the admin closes that report as ACTIONED with the note "đã gỡ tin"
    Then the close call fails with "unavailable"
    And the open reports for "L1" still contain the report
    When team-audit is started again
    And the admin closes that report as ACTIONED with the note "đã gỡ tin"
    Then the close call succeeds with status ACTIONED

  Scenario: Listing reports: Buyer cannot work the queue end to end
    Given a seller with a published listing "L1"
    And a buyer "b1"
    And an admin
    And "b1" has reported "L1" with reason "scam"
    When "b1" lists reports for "L1"
    Then the list call fails with "permission_denied"
    When "b1" closes that report as ACTIONED with the note "self service"
    Then the close call fails with "permission_denied"
    And the open reports for "L1" still contain the report
    When the admin lists open reports for "L1"
    Then the list contains the report of "b1"
