@order
Feature: GetUserObligations counts a user's open orders and returns for the identity service
  OrderService.GetUserObligations is service-only (scope order.internal.read) and not routed at
  the gateway. It is called from the stack network with a service principal; seeding goes through
  the gateway. (change commerce-user-erasure)

  Scenario: OBL-1 Open orders and returns are counted
    Given a seller "S" with a stocked listing "L"
    And a user "U" who is both a buyer and a seller with a stocked listing "LU"
    And a buyer "B"
    And "U" is buyer of 2 pending orders of "L"
    And "U" is buyer of 1 shipped order of "L"
    And "U" is seller of 1 paid order of "LU" bought by "B"
    And "U" has a pending return on its shipped order
    When the identity service calls GetUserObligations for "U"
    Then the obligations are open buyer orders 3, open seller orders 1, open returns 1

  Scenario: OBL-2 Closed orders and returns are not counted
    Given a seller "S" with a stocked listing "L"
    And a buyer "U"
    And "U" is buyer of 1 completed order of "L"
    And "U" is buyer of 1 cancelled order of "L"
    And "U" has a rejected return on its completed order
    And "U" has a refunded return on an online-paid completed order of "L"
    When the identity service calls GetUserObligations for "U"
    Then the obligations are open buyer orders 0, open seller orders 0, open returns 0

  Scenario: OBL-3 A user with no data reads zeros
    Given a user "N" with no orders, returns, wallet entries or payouts
    When the identity service calls GetUserObligations for "N"
    Then the response is OK with all three counts 0

  Scenario: OBL-7 A user principal is refused
    Given a buyer "U"
    When a signed-in user principal calls GetUserObligations for "U"
    Then the call fails with "permission_denied"

  Scenario: OBL-8 A service principal without the scope is refused
    Given a buyer "U"
    When a service principal holding only order.read calls GetUserObligations for "U"
    Then the call fails with "permission_denied"

  Scenario: OBL-9 An anonymous caller is refused
    Given a buyer "U"
    When a call without a principal reaches GetUserObligations for "U"
    Then the call fails with "unauthenticated"

  Scenario: OBL-10 The gateway does not route the RPC
    Given a buyer "U"
    When a browser client calls OrderService/GetUserObligations through the gateway with a token
    Then the gateway answers HTTP 501
    When a browser client calls OrderService/GetUserObligations through the gateway without a token
    Then the gateway answers HTTP 501
