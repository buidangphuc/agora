@promo
Feature: Promotion admin operations need the promotion.admin scope
  team-identity issues promotion.admin to the admin role only. The gateway gates the five promotion
  admin procedures on promotion.admin; team-promotion requires admin and promotion.admin on its
  admin-only RPCs and takes the admin branch of mixed RPCs only with promotion.admin. Tokens without
  it are signed with the local dev identity key. Change: promotion-admin-scope (auth,
  edge-route-policy, promotion-access-control). Needs the rebuilt team-identity, team-promotion and
  team-gateway images.

  Scenario: An admin token carries promotion.admin
    Given the seeded admin for the promotion-scope checks
    Then the seeded admin token carries "admin" and "promotion.admin"

  Scenario: Buyer and seller tokens never carry promotion.admin
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    Then neither token of "sa" and "b1" carries "promotion.admin" or "admin"

  Scenario: Promotion admin edge: a token with admin but without promotion.admin is refused
    When a caller whose token carries "admin,listing.write" calls ListCampaigns and DeactivateVoucher
    Then both promotion calls are answered with HTTP 403

  Scenario: Promotion admin edge: an anonymous caller is refused
    When a request without a token calls CancelCampaign
    Then the promotion call is answered with HTTP 401

  Scenario: Promotion admin edge: the seeded admin lists campaigns
    Given the seeded admin for the promotion-scope checks
    When the seeded admin calls ListCampaigns
    Then the promotion call succeeds with a campaigns list

  Scenario: Promotion admin scope: the admin deactivates a platform voucher
    Given the seeded admin for the promotion-scope checks
    And the seeded admin has created a platform voucher
    When the seeded admin calls DeactivateVoucher on that voucher
    Then the promotion call succeeds
    And that voucher is inactive

  Scenario: Promotion admin scope: promotion.admin without admin is denied by the service
    Given the seeded admin for the promotion-scope checks
    And the seeded admin has created a platform voucher
    When a caller whose token carries "promotion.admin" calls DeactivateVoucher on that voucher
    Then the promotion call fails with "permission_denied"
    And that voucher is still active

  Scenario: Promotion admin scope: admin without promotion.admin cannot create a platform voucher
    Given the seeded admin for the promotion-scope checks
    When a caller whose token carries "admin,listing.write" creates a "VOUCHER_SCOPE_PLATFORM" voucher
    Then the promotion call fails with "permission_denied"
    And GetVoucher for that code returns not found

  Scenario: Promotion admin scope: sellers and anonymous callers are refused
    Given a seller "sa" who owns a listing "L" with stock 10
    When "sa" calls CancelCampaign and a request without a token calls UpdateVoucher
    Then the gateway answers HTTP 403 to the seller and HTTP 401 to the anonymous request

  Scenario: Promotion admin scope: admin without promotion.admin creates a shop voucher as a seller
    Given the seeded admin for the promotion-scope checks
    When a caller whose token carries "admin,listing.write" creates a "VOUCHER_SCOPE_SHOP" voucher
    Then the promotion call succeeds
    And the created voucher belongs to that caller
    And the audit log holds no "voucher.admin_create" event for that code

  Scenario: Promotion admin scope: admin without promotion.admin cannot run a flash sale on another seller's listing
    Given a seller "sa" who owns a listing "L" with stock 10
    And the seeded admin for the promotion-scope checks
    When a caller whose token carries "admin,listing.write" creates a flash sale for "L"
    Then the promotion call fails with "permission_denied"
    And the seeded admin lists no campaign for "L"

  Scenario: Promotion admin scope: admin without promotion.admin cannot advertise another seller's listing
    Given a seller "sa" who owns a listing "L" with stock 10
    When a caller whose token carries "admin,listing.write" creates an ad campaign for "L"
    Then the promotion call fails with "permission_denied"

  Scenario: Promotion admin scope: admin without promotion.admin or listing.write is denied a shop voucher
    When a caller whose token carries "admin" creates a "VOUCHER_SCOPE_SHOP" voucher
    Then the promotion call fails with "permission_denied"
