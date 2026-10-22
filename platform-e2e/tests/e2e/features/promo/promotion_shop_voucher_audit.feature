@promo
Feature: Admin-created shop vouchers are audited and fail closed
  A SHOP voucher created by an admin leaves one team-audit event written before the voucher
  row. When the audit trail cannot be written the admin call is refused and nothing is
  created. A seller's own shop voucher is not audited and does not depend on team-audit.

  Scenario: Shop voucher audit: admin creates a shop voucher
    Given the seeded admin for the shop voucher audit check
    When the admin creates a SHOP voucher with a fresh code, discount type percent and value 10
    Then the shop voucher call succeeds
    And the shop voucher exists
    And the audit log holds exactly one "voucher.admin_create" event for the shop voucher by the admin with discount type "DISCOUNT_TYPE_PERCENT", value "10" and the admin id as seller id

  @destructive
  Scenario: Shop voucher audit: seller's own shop voucher is not audited
    Given the seeded admin for the shop voucher audit check
    And a seller for the shop voucher audit check
    And team-audit is stopped for the shop voucher audit check
    When the seller creates a SHOP voucher with a fresh code for the shop voucher audit check
    Then the shop voucher call succeeds
    And the audit log holds no "voucher.admin_create" event for the shop voucher after team-audit is started again
    And an admin SHOP voucher created now is audited as a positive control

  @destructive
  Scenario: Shop voucher audit: audit down blocks an admin shop voucher
    Given the seeded admin for the shop voucher audit check
    And team-audit is stopped for the shop voucher audit check
    When the admin creates a SHOP voucher with a fresh code, discount type percent and value 10
    Then the shop voucher call fails with code "unavailable"
    And the shop voucher is not found after team-audit is started again
    And an admin SHOP voucher created now is audited as a positive control
