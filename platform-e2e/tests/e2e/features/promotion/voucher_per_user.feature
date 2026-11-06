@promo
Feature: A user-bound voucher is usable once, only by its owner
  ValidateAndReserve refuses a voucher bound to another buyer, enforces the per-user limit and
  refuses new-customer-only vouchers for a buyer with a completed order. The service-only RPC is
  driven as the order service.

  Scenario: Another buyer cannot use a user-bound voucher
    Given "alice" registered through the gateway with no referral code and holds a voucher
    And "bob" registered through the gateway with no referral code and holds a voucher
    When "bob" reserves the voucher of "alice" as the order service with subtotal 200000
    Then the reservation is refused with reason "VOUCHER_NOT_ELIGIBLE" and no error
    And "alice" still lists her voucher with nothing used

  Scenario: The owner uses the voucher once
    Given "alice" registered through the gateway with no referral code and holds a voucher
    When "alice" reserves her own voucher as the order service with subtotal 200000
    Then the reservation is valid with discount 20000
    When the order service commits the first reservation
    And "alice" reserves her own voucher as the order service with subtotal 200000
    Then the reservation is refused with reason "VOUCHER_PER_USER_LIMIT" and no error
    And "alice" no longer lists the voucher

  Scenario: new_user_only refuses a buyer with a completed order
    Given a seller with a stocked listing for new-user voucher orders
    And a shop voucher of that seller limited to one use per buyer and to buyers without a completed order
    And a buyer "veteran" with a completed order
    And a buyer "newbie" with no completed order
    When "veteran" reserves the shop voucher as the order service with subtotal 200000
    Then the reservation is refused with reason "VOUCHER_NOT_ELIGIBLE" and no error
    When "newbie" reserves the shop voucher as the order service with subtotal 200000
    Then the reservation is valid
