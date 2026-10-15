@promo
Feature: Admin promotions are audited and fail closed
  A platform voucher, and a flash sale or ad campaign created by an admin, each leave one
  team-audit event written before the change. When the audit trail cannot be written the
  admin call is refused and nothing is created. A seller's own promotions are not audited and
  do not depend on team-audit.

  Scenario: Promotion audit: Admin creates a platform voucher
    Given the seeded admin for the promotion audit check
    When the admin creates a PLATFORM voucher with a fresh code, discount type percent and value 10
    Then the promotion call succeeds
    And the voucher exists
    And the audit log holds exactly one "voucher.platform_create" event for the voucher by the admin with discount type "DISCOUNT_TYPE_PERCENT" and value "10"

  Scenario: Promotion audit: Admin creates a flash sale with an audit event
    Given the seeded admin for the promotion audit check
    And a promotion seller with a published listing
    When the admin creates a flash-sale campaign on that listing
    Then the promotion call succeeds
    And the audit log holds exactly one "flash_sale.admin_create" event for the returned campaign by the admin with the listing id

  Scenario: Promotion audit: Admin creates an ad campaign with an audit event
    Given the seeded admin for the promotion audit check
    And a promotion seller with a published listing
    When the admin creates an ad campaign with budget 50000 and bid 1000 on that listing
    Then the promotion call succeeds
    And the audit log holds exactly one "ad_campaign.admin_create" event for the returned campaign by the admin with the listing id, the stored seller id and budget "50000"

  @destructive
  Scenario: Promotion audit: Seller creates a flash sale on their own listing
    Given a promotion seller with a published listing
    And team-audit is stopped for the promotion audit check
    When the seller creates a flash-sale campaign on their own listing
    Then the promotion call succeeds
    And the audit log holds no "flash_sale.admin_create" event for the returned campaign after team-audit is started again

  @destructive
  Scenario: Promotion audit: Seller creates an ad campaign on their own listing
    Given a promotion seller with a published listing
    And team-audit is stopped for the promotion audit check
    When the seller creates an ad campaign on their own listing
    Then the promotion call succeeds
    And the audit log holds no "ad_campaign.admin_create" event for the returned campaign after team-audit is started again

  @destructive
  Scenario: Promotion audit: Shop voucher by a seller writes no event
    Given a promotion seller with a published listing
    And team-audit is stopped for the promotion audit check
    When the seller creates a SHOP voucher with a fresh code
    Then the promotion call succeeds
    And the audit log holds no "voucher.platform_create" event for the voucher after team-audit is started again

  @destructive
  Scenario: Promotion audit: Audit down blocks a platform voucher
    Given the seeded admin for the promotion audit check
    And team-audit is stopped for the promotion audit check
    When the admin creates a PLATFORM voucher with a fresh code, discount type percent and value 10
    Then the promotion call fails with code "unavailable"
    And the voucher is not found

  @destructive
  Scenario: Promotion audit: Audit down blocks an admin flash sale
    Given the seeded admin for the promotion audit check
    And a promotion seller with a published listing
    And team-audit is stopped for the promotion audit check
    When the admin creates a flash-sale campaign on that listing
    Then the promotion call fails with code "unavailable"
    And the listing has no active flash sale

  @destructive
  Scenario: Promotion audit: Audit down blocks an admin ad campaign
    Given the seeded admin for the promotion audit check
    And a promotion seller with a published listing
    And team-audit is stopped for the promotion audit check
    When the admin creates an ad campaign with budget 50000 and bid 1000 on that listing
    Then the promotion call fails with code "unavailable"
    And the listing is absent from the sponsored slots
