@promo
Feature: A buyer lists only their own vouchers and public reads never expose user-bound ones
  ListMyVouchers is routed for any signed-in principal, the public list excludes user-bound
  vouchers, and HasCompletedOrder stays internal.

  Scenario: A signed-in buyer sees only their own vouchers
    Given "alice" registered through the gateway with no referral code and holds a voucher
    And "bob" registered through the gateway with no referral code and holds a voucher
    When "alice" lists her vouchers through the gateway
    Then the list has exactly one voucher and it is owned by "alice"
    And no listed voucher belongs to "bob"

  Scenario: An anonymous caller cannot list vouchers
    When an anonymous caller lists vouchers through the gateway
    Then the gateway answers UNAUTHENTICATED

  Scenario: Public list excludes user-bound vouchers
    Given "alice" registered through the gateway with no referral code and holds a voucher
    And "bob" registered through the gateway with no referral code and holds a voucher
    And "carol" registered through the gateway with no referral code and holds a voucher
    When an anonymous visitor reads the public voucher list through the gateway
    Then no public voucher is user-bound and none of those new-user vouchers is listed

  Scenario: HasCompletedOrder is not routed at the gateway
    Given a signed-in buyer "alice"
    When "alice" posts HasCompletedOrder to the gateway
    Then the gateway answers UNIMPLEMENTED with HTTP 404 or 501
