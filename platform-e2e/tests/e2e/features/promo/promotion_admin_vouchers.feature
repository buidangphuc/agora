@promo
Feature: Admin stops and edits platform vouchers, with clear errors (promotion-admin-vouchers)
  An admin stops a platform voucher or moves its end date and raises its quota through the
  gateway, every admin voucher write is audited first and fails closed, a taken code is
  ALREADY_EXISTS, and audit-down differs from promotion-down. Scenario names echo the spec
  scenarios of openspec/changes/promotion-admin-vouchers. Every refusal is paired with a
  positive control (the same admin can still stop the voucher, or the reservation was valid
  before) so it cannot pass vacuously.

  Scenario: V1 stop blocks new checkouts
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin stops the voucher
    Then the call succeeds
    And the returned voucher is inactive with a deactivated_at

  Scenario: V1a next reservation is refused
    Given the seeded admin for the voucher admin checks
    And a registered buyer for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And a control reservation of the voucher is accepted
    And the admin has stopped the voucher
    When the buyer previews the voucher through the gateway
    Then the last reservation is refused with reason "inactive"

  Scenario: V1b no hold is created
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And a control reservation of the voucher is accepted
    And the admin has stopped the voucher
    When a reservation of the voucher is requested
    Then the last reservation is refused with reason "inactive"
    And no reservation row exists for the refused reservation

  Scenario: V1c inactive wins over expired
    Given the seeded admin for the voucher admin checks
    And an active platform voucher whose end date has passed
    And a control reservation of the voucher is refused for another reason than inactive
    And the admin has stopped the voucher
    When a reservation of the voucher is requested
    Then the last reservation is refused with reason "inactive"

  Scenario: V2 existing hold still commits
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an order holds the voucher under a reservation
    And the admin has stopped the voucher
    When the held reservation is committed
    Then the commit reports committed and the voucher used count is 1

  Scenario: V3 existing hold still releases
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an order holds the voucher under a reservation
    And the admin has stopped the voucher
    When the held reservation is released
    Then the release reports released and the voucher used count is 0

  Scenario: V4 stopping twice is a no-op
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And the admin has stopped the voucher
    When the admin stops the voucher
    Then the call succeeds
    And the returned voucher keeps the original deactivated_at

  Scenario: V4a stopping twice is audited per call
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And the admin has stopped the voucher
    When the admin stops the voucher
    Then the call succeeds
    And the audit log holds exactly one more "voucher.deactivate" event for the voucher

  Scenario: V5 unknown voucher is NOT_FOUND
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin stops a voucher with an unknown id
    Then the call fails with code "not_found"
    And the admin can still stop the voucher

  Scenario: V5a unknown voucher writes no audit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin stops a voucher with an unknown id
    Then the call fails with code "not_found"
    And no "voucher.deactivate" audit entry mentions the unknown voucher id
    And the admin can still stop the voucher

  Scenario: V6 non-admin cannot stop
    Given the seeded admin for the voucher admin checks
    And a registered seller for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the seller stops the voucher
    Then the call fails with code "permission_denied"
    And the admin can still stop the voucher

  Scenario: V6a non-admin stop leaves no trace
    Given the seeded admin for the voucher admin checks
    And a registered buyer for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the buyer stops the voucher
    Then the call fails with code "permission_denied"
    And the audit log holds no "voucher.deactivate" event for the voucher
    And the stored voucher is unchanged
    And the admin can still stop the voucher

  Scenario: V7 anonymous cannot stop or update
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When an anonymous caller stops the voucher
    And an anonymous caller updates the voucher with quota 9
    Then every call fails with code "unauthenticated"
    And the stored voucher is unchanged
    And the admin can still stop the voucher

  Scenario: V7b buyer token cannot stop or update through the gateway
    Given the seeded admin for the voucher admin checks
    And a registered buyer for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the buyer stops the voucher
    And the buyer updates the voucher with quota 9
    Then every call fails with code "permission_denied"
    And the stored voucher is unchanged
    And the admin can still stop the voucher

  Scenario: V8 raise quota
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 2 used
    When the admin updates the voucher with quota 8
    Then the call succeeds
    And the returned voucher has quota 8

  Scenario: V8a raise quota leaves other fields
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with quota 8
    Then the call succeeds
    And the returned voucher keeps its money fields and end date

  Scenario: V8b raise quota is audited once
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with quota 8
    Then the call succeeds
    And the audit log holds exactly 1 "voucher.update" event for the voucher

  Scenario: V9 move end date
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with an end date 60 days from now
    Then the call succeeds
    And the returned voucher has the new end date

  Scenario: V9a new end date reopens the window
    Given the seeded admin for the voucher admin checks
    And an active platform voucher whose end date has passed
    And a control reservation of the voucher is refused for another reason than inactive
    When the admin updates the voucher with an end date 5 days from now
    And a reservation of the voucher is requested
    Then the last reservation is accepted

  Scenario: V9b ends_at at or before now is refused
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with an end date -3600 seconds from now
    And the admin updates the voucher with an end date 0 seconds from now
    Then every call fails with code "invalid_argument"
    And the stored voucher is unchanged
    And the admin can still update the voucher with an end date

  Scenario: V9c ends_at one second after now is accepted
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with an end date 1 seconds from now
    Then the call succeeds
    And the returned voucher has the new end date

  Scenario: V10 quota below used is refused
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 2 used
    When the admin updates the voucher with quota 1
    Then the call fails with code "invalid_argument"
    And the stored voucher is unchanged
    And the admin can still update the voucher with quota 6

  Scenario: V10a refused quota writes no audit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 2 used
    When the admin updates the voucher with quota 1
    Then the call fails with code "invalid_argument"
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 6

  Scenario: V11 quota lowered is refused
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 10 and 2 used
    When the admin updates the voucher with quota 6
    Then the call fails with code "invalid_argument"
    And the stored voucher is unchanged
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 11

  Scenario: V12 fully used voucher with the same quota and a new end date is accepted
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 2 and 2 used
    When the admin updates the voucher with the current quota and an end date 60 days from now
    Then the call succeeds

  Scenario: V12a same quota and no end date is nothing to change
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with the current quota and no end date
    Then the call fails with code "invalid_argument"
    And the error message says "nothing to change"
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 6

  Scenario: V12b quota equal to current with ends_at
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with the current quota and an end date 60 days from now
    Then the call succeeds
    And the returned voucher has the new end date
    And the returned voucher has quota 5

  Scenario: V12c quota equal to current with ends_at is audited once
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with the current quota and an end date 60 days from now
    Then the call succeeds
    And the audit log holds exactly 1 "voucher.update" event for the voucher

  Scenario: V12d quota greater than used and greater than current
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 2 and 2 used
    When the admin updates the voucher with quota 3
    Then the call succeeds
    And the returned voucher has quota 3

  Scenario: V13 end date not after start is refused
    Given the seeded admin for the voucher admin checks
    And an active platform voucher that starts in 2 days and ends in 10 days
    When the admin updates the voucher with an end date equal to the start date
    And the admin updates the voucher with an end date one second before the start date
    Then every call fails with code "invalid_argument"
    And the audit log holds no "voucher.update" event for the voucher
    And the stored voucher is unchanged
    And the admin can still update the voucher with an end date

  Scenario: V14 nothing to change
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with neither quota nor end date
    Then the call fails with code "invalid_argument"
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 6

  Scenario: V15 inactive voucher cannot be edited
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And the admin has stopped the voucher
    When the admin updates the voucher with quota 8
    Then the call fails with code "failed_precondition"
    And the audit log holds no "voucher.update" event for the voucher
    And the stored voucher is unchanged

  Scenario: V15a unknown voucher on update
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates an unknown voucher id with quota 8
    Then the call fails with code "not_found"
    And no "voucher.update" audit entry mentions the unknown voucher id
    And the admin can still update the voucher with quota 6

  Scenario: V16 money fields are never altered by an update
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 1 used
    When the admin updates the voucher with quota 9 and an end date 60 days from now
    Then the call succeeds
    And the returned voucher keeps its money fields
    And the stored voucher keeps its money fields

  Scenario: V16a quota zero is refused
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin updates the voucher with quota 0
    Then the call fails with code "invalid_argument"
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 6

  Scenario: V16b unlimited voucher cannot become finite
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with unlimited quota
    When the admin updates the voucher with quota 500
    Then the call fails with code "invalid_argument"
    And the stored voucher has quota 0
    And the admin can still update the voucher with an end date

  Scenario: V16c unlimited voucher end date can move
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with unlimited quota
    When the admin updates the voucher with an end date 60 days from now
    Then the call succeeds
    And the returned voucher has the new end date
    And the returned voucher has quota 0

  Scenario: V17 non-admin cannot edit
    Given the seeded admin for the voucher admin checks
    And a registered seller for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the seller updates the voucher with quota 8
    Then the call fails with code "permission_denied"
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still update the voucher with quota 6

  Scenario: V18 taken code is refused with an error
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin creates a platform voucher with the same code
    Then the call fails with code "already_exists"
    And the error message names the code field

  Scenario: V18a duplicate writes no audit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin creates a platform voucher with the same code
    Then the call fails with code "already_exists"
    And the audit log holds exactly 1 "voucher.platform_create" event for the voucher
    And the stored voucher is unchanged

  Scenario: V18b duplicate race has one winner
    Given the seeded admin for the voucher admin checks
    When two admins create the same new platform code at once
    Then exactly one of the calls succeeds

  Scenario: V18c duplicate race loser gets ALREADY_EXISTS
    Given the seeded admin for the voucher admin checks
    When two admins create the same new platform code at once
    Then exactly one of the calls succeeds
    And the failing call returns "already_exists", never "internal"

  Scenario: V19 gateway keeps the code
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the admin creates a platform voucher with the same code
    Then the gateway answers with Connect code "already_exists" and not "internal"

  @destructive
  Scenario: V21 audit down on any admin voucher write
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-audit is stopped for the voucher admin checks
    When the admin creates a new platform voucher, stops the voucher and updates its quota to 8
    Then every call fails with code "unavailable"
    And every error carries reason "AUDIT_UNAVAILABLE" and domain "promotion.platform"

  @destructive
  Scenario: V21a audit down changes nothing
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-audit is stopped for the voucher admin checks
    When the admin creates a new platform voucher, stops the voucher and updates its quota to 8
    Then every call fails with code "unavailable"
    And after team-audit is started again nothing was created or changed

  @destructive
  Scenario: V21b audit down with invalid input
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-audit is stopped for the voucher admin checks
    When the admin updates the voucher with quota 0 while audit is down
    And the admin creates a platform voucher with the same code
    And the admin stops a voucher with an unknown id
    Then the calls fail with "invalid_argument", "already_exists" and "not_found" respectively
    And after team-audit is started again nothing was created or changed

  @destructive
  Scenario: V23 gateway keeps the audit reason
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-audit is stopped for the voucher admin checks
    When the admin stops the voucher
    Then the call fails with code "unavailable"
    And the error carries reason "AUDIT_UNAVAILABLE" and domain "promotion.platform"
    And the error message is the generic "service unavailable" text

  @destructive
  Scenario: V23a gateway never forwards the raw backend message
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-audit is stopped for the voucher admin checks
    When the admin stops the voucher
    Then the call fails with code "unavailable"
    And the error carries reason "AUDIT_UNAVAILABLE" and domain "promotion.platform"
    And the error message does not contain the upstream audit text

  @destructive
  Scenario: V24 gateway reports promotion down
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And team-promotion is stopped for the voucher admin checks
    When the admin stops the voucher
    Then the call fails with code "unavailable"
    And the error carries reason "PROMOTION_UNAVAILABLE" and domain "promotion.platform"

  @destructive
  Scenario: V25 upstream unavailable without ErrorInfo stays generic
    Given the seeded admin for the voucher admin checks
    And team-audit is stopped for the voucher admin checks
    When the admin queries the audit log through the gateway
    Then the call fails with code "unavailable"
    And the error carries no ErrorInfo reason

  Scenario: V27a non-admin write is denied before audit
    Given the seeded admin for the voucher admin checks
    And a registered buyer for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    When the buyer creates a platform voucher, stops the voucher and updates its quota to 8
    Then every call fails with code "permission_denied"
    And the buyer's new voucher does not exist
    And the audit log holds no "voucher.deactivate" event for the voucher
    And the audit log holds no "voucher.update" event for the voucher
    And the admin can still stop the voucher

  Scenario: V34 gaps removed
    Given the gaps document of the change
    Then the gaps document has no row whose id is G9, G10 or G11

  Scenario: V35 other gaps untouched
    Given the gaps document of the change
    Then the gaps document differs from the base only by the removal of rows G9, G10 and G11
    And every other row is byte-identical and every row has the table format of the header
