@order
Feature: team-order answers whether a buyer has a completed order, to internal callers only
  OrderService.HasCompletedOrder returns has_completed = true iff the buyer has at least one order
  in status Completed, whoever completed it. Only a SERVICE principal holding order.read may call
  it; users, admins and scope-less services are refused, and the gateway never routes it.
  (order-has-completed-order, initiative q7-q37-referral-welcome)

  Scenario: Buyer with a completed order
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b1"
    And "b1" has a completed order for "L"
    When a service principal with order.read asks whether "b1" has a completed order
    Then the lookup succeeds with has_completed true

  Scenario: Buyer with a completed and a cancelled order
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b1"
    And "b1" has a completed order for "L"
    And "b1" has a cancelled order for "L"
    When a service principal with order.read asks whether "b1" has a completed order
    Then the lookup succeeds with has_completed true

  Scenario: Buyer with only non-completed orders
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b2"
    And "b2" has a pending order for "L"
    And "b2" has a paid order for "L"
    And "b2" has a shipped order for "L"
    And "b2" has a cancelled order for "L"
    When a service principal with order.read asks whether "b2" has a completed order
    Then the lookup succeeds with has_completed false

  Scenario: Buyer with no orders
    Given a buyer "b3"
    When a service principal with order.read asks whether "b3" has a completed order
    Then the lookup succeeds with has_completed false

  Scenario: Another buyer's completed order does not count
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has a completed order for "L"
    When a service principal with order.read asks whether "b2" has a completed order
    Then the lookup succeeds with has_completed false

  Scenario: Completed by seller or system counts
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b1"
    And "b1" has a shipped order for "L"
    When the seller "sa" completes the shipped order of "b1"
    And a service principal with order.read asks whether "b1" has a completed order
    Then the lookup succeeds with has_completed true

  Scenario: Status change is visible on the next call
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b2"
    And "b2" has a shipped order for "L"
    When a service principal with order.read asks whether "b2" has a completed order
    Then the lookup succeeds with has_completed false
    When "b2" confirms receipt of the shipped order
    And a service principal with order.read asks whether "b2" has a completed order
    Then the lookup succeeds with has_completed true

  Scenario: Empty buyer id
    When a service principal with order.read asks about an empty buyer id
    Then the lookup fails with "invalid_argument"

  Scenario: Buyer principal refused
    Given a seller "sa" with a stocked listing "L"
    And a buyer "b1"
    And "b1" has a completed order for "L"
    When the user principal "b1" asks whether "b1" has a completed order
    Then the lookup fails with "permission_denied"
    When the user principal "sa" asks whether "b1" has a completed order
    Then the lookup fails with "permission_denied"

  Scenario: Admin refused
    Given a buyer "b1"
    When a user principal holding order.admin asks whether "b1" has a completed order
    Then the lookup fails with "permission_denied"

  Scenario: Service without the scope refused
    Given a buyer "b1"
    When a service principal without order.read asks whether "b1" has a completed order
    Then the lookup fails with "permission_denied"

  Scenario: Unauthenticated call refused
    Given a buyer "b1"
    When a call without any principal asks whether "b1" has a completed order
    Then the lookup fails with "unauthenticated"

  Scenario: Not routed at the gateway
    Given a buyer "b1"
    When "b1" posts HasCompletedOrder for "b1" through the gateway
    Then the gateway answers HTTP 501 with code unimplemented
    When the admin posts HasCompletedOrder for "b1" through the gateway
    Then the gateway answers HTTP 501 with code unimplemented
