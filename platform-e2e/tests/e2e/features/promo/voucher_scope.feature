@promo
Feature: ValidateAndReserve reports the voucher scope and its shop
  VoucherService.ValidateAndReserve of team-promotion echoes the stored scope and seller_id of the
  voucher it was asked about (change promotion-voucher-scope, initiative q11-checkout-coherence), so
  the order service can route a code to a shop group or to the platform slot. The call is
  service-only (promotion.reserve) apart from a buyer's own preview namespace.

  Scenario: Shop voucher reports SHOP scope and its seller
    Given a seller with a shop voucher
    When the order service reserves the shop voucher for its own seller
    Then the reserve is valid with scope "VOUCHER_SCOPE_SHOP" and the seller of the voucher
    And a hold exists for the reservation

  Scenario: Platform voucher reports PLATFORM scope and no seller
    Given a platform voucher with minimum spend 0
    When the order service reserves the platform voucher with an empty seller id and subtotal 100000
    Then the reserve is valid with scope "VOUCHER_SCOPE_PLATFORM" and no seller

  Scenario: Platform voucher min spend is checked on the subtotal sent
    Given a platform voucher with minimum spend 500000
    When the order service reserves the platform voucher with subtotal 499999
    And the order service reserves the platform voucher again under a new reservation id with subtotal 500000
    Then the first reserve is refused with reason "order subtotal below minimum spend"
    And the second reserve is valid
    And both reserves report scope "VOUCHER_SCOPE_PLATFORM"

  Scenario: A shop voucher for another seller is refused and still reports its shop
    Given a seller with a shop voucher
    And another seller
    When the order service reserves the shop voucher for its own seller
    And the order service reserves the shop voucher for the other seller under a new reservation id
    Then the first reserve is valid with scope "VOUCHER_SCOPE_SHOP" and the seller of the voucher
    And the second reserve is refused with reason "voucher not valid for this seller"
    And the second reserve reports scope "VOUCHER_SCOPE_SHOP" and the seller of the voucher
    And no hold exists for the second reservation

  Scenario: An unknown code reports no scope
    Given a platform voucher with minimum spend 0
    When the order service reserves the platform voucher with an empty seller id and subtotal 100000
    And the order service reserves a code that does not exist
    And the order service reserves an empty code
    Then the reserve of the existing voucher reports scope "VOUCHER_SCOPE_PLATFORM"
    And the unknown code is refused with reason "voucher code not found" and no scope
    And the empty code is refused with reason "voucher code is required" and no scope

  Scenario: A buyer preview receives the scope
    Given a registered voucher buyer
    And a platform voucher with minimum spend 0
    When the buyer reserves the platform voucher under their own preview namespace
    Then the reserve is valid with scope "VOUCHER_SCOPE_PLATFORM" and no seller
    And the hold is bound to the buyer id of the principal

  Scenario: A buyer outside the preview namespace is denied
    Given a registered voucher buyer
    And a platform voucher with minimum spend 0
    When the buyer reserves the platform voucher under their own preview namespace
    And the buyer reserves the platform voucher under another user's preview namespace
    And the buyer reserves the platform voucher with a reservation id that has no preview prefix
    Then the reserve under their own namespace is valid with scope "VOUCHER_SCOPE_PLATFORM"
    And the reserve under another user's namespace is denied and no hold exists
    And the reserve without a preview prefix is denied and no hold exists
