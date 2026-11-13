@promo
Feature: team-promotion releases a user's vouchers on UserErased
  A UserErased event on identity.events deactivates the vouchers the user owns. The user's welcome
  voucher is read through ListMyVouchers and tried at checkout.
  (change commerce-user-erasure)

  Scenario: PRO-1 Owned vouchers are deactivated and state is deleted
    Given "U" registered through the gateway and holds an active welcome voucher
    When a UserErased event for "U" is consumed
    Then the welcome voucher of "U" is no longer active

  Scenario: PRO-8 A deactivated voucher cannot be redeemed
    Given a seller with a stocked listing for voucher checkouts
    And "U" registered through the gateway and holds an active welcome voucher
    When a UserErased event for "U" is consumed
    Then the welcome voucher of "U" is no longer active
    And "U" cannot check out with that voucher and no order is created
