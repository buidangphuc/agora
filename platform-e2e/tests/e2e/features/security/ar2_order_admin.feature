@order
Feature: Admin order operations need the order.admin scope
  team-identity issues order.admin to the admin role only. team-order requires it, together with
  admin, for ForceFailSaga, and recognises an admin on its other order RPCs by order.admin alone.
  Tokens without it are signed with the local dev identity key. Change: authz-residuals-2
  (auth, order-read-access). Needs the rebuilt team-identity and team-order images.

  Scenario: An admin token carries order.admin
    Given the seeded admin for the order-scope checks
    Then the admin's token scopes include "admin" and "order.admin"

  Scenario: Buyer and seller tokens carry no order.admin
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    Then the token scopes of "sa" and "b1" include neither "order.admin" nor "admin"

  Scenario: An admin token without order.admin cannot force-fail
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When a caller whose token carries the admin scope but not order.admin calls ForceFailSaga on the order
    Then the order call fails with "permission_denied"
    And the order read by "b1" remains Pending
    And the stock of "L" is unchanged at 8

  Scenario: An admin token without order.admin cannot read another user's saga
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When a caller whose token carries the admin scope but not order.admin calls GetSagaState on the order
    Then the order call fails with "permission_denied"

  Scenario: The admin can still read another user's saga
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    And the seeded admin for the order-scope checks
    When the seeded admin calls GetSagaState on the order
    Then the saga response is for that order

  Scenario: A buyer cannot force-fail another buyer's order
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has a Pending order for 2 of "L"
    When the buyer "b2" calls ForceFailSaga on the order of "b1" through the gateway
    Then the gateway answers HTTP 403 to the force-fail call
    And the order read by "b1" remains Pending
