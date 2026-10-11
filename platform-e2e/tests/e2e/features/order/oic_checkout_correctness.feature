@order
Feature: Checkout is all-or-nothing, per-attempt and idempotent on the client's key
  The team-order checkout reserves every seller group's stock before creating any order,
  gives each attempt its own reservations and honours the Idempotency-Key the gateway forwards.
  (port-order-inventory-correctness / order-checkout-correctness, storefront requirement excluded)

  Scenario: One seller out of stock fails the whole two-seller checkout
    Given a seller "sa" who owns a listing "LA" with stock 5
    And a seller "sb" who owns a listing "LB" with stock 3
    And a buyer "b1"
    And "b1" has 1 of "LA" in the cart
    And "b1" has 4 of "LB" in the cart
    When "b1" checks out
    Then the call fails with "resource_exhausted"
    And "b1" has 0 orders
    And the stock of "LA" is back to 5
    And the cart of "b1" still holds 2 items

  Scenario: A two-seller checkout places one order per seller
    Given a seller "sa" who owns a listing "LA" with stock 5
    And a seller "sb" who owns a listing "LB" with stock 5
    And a buyer "b1"
    And "b1" has 2 of "LA" in the cart
    And "b1" has 1 of "LB" in the cart
    When "b1" checks out
    Then 2 Pending orders are returned, one per seller "sa" and "sb"
    And the stock of "LA" is 3
    And the stock of "LB" is 4
    And the cart of "b1" is empty

  Scenario: An unkeyed retry after a failed checkout succeeds
    Given a seller "sa" who owns a listing "LA" with stock 5
    And a seller "sb" who owns a listing "LB" with stock 3
    And a buyer "b1"
    And "b1" has 1 of "LA" in the cart
    And "b1" has 4 of "LB" in the cart
    When "b1" checks out
    Then the call fails with "resource_exhausted"
    When "sb" restocks "LB" to 10
    And "b1" checks out again without a key
    Then the second checkout returns one order per seller
    And the stock of "LA" is 4

  Scenario: The same idempotency key returns the same orders
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has 2 of "L" in the cart
    When "b1" checks out twice with the idempotency key "k1"
    Then both calls return the same order ids
    And "b1" has exactly one new order
    And the stock of "L" is 8

  Scenario: Concurrent checkouts with one key create one set of orders
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has 2 of "L" in the cart
    When "b1" sends two simultaneous checkouts with the idempotency key "k1"
    Then each call returns the same order ids or fails with aborted and at least one returns orders
    And "b1" has exactly one new order
    And the stock of "L" is 8

  Scenario: A failed checkout frees its idempotency key
    Given a seller "sa" who owns a listing "L" with stock 2
    And a buyer "b1"
    And "b1" has 3 of "L" in the cart
    When "b1" checks out with the idempotency key "k1"
    Then the call fails with "resource_exhausted"
    When "sa" restocks "L" to 10
    And "b1" checks out with the idempotency key "k1"
    Then the call succeeds
    And "b1" has exactly one new order

  Scenario: Idempotency keys are scoped to the buyer
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has 1 of "L" in the cart
    And "b2" has 1 of "L" in the cart
    When "b1" and "b2" each check out with the idempotency key "shared"
    Then each buyer receives their own new order
    And "b1" has exactly one new order
    And "b2" has exactly one new order
