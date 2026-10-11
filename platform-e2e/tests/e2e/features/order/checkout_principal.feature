@order
Feature: Only a user can place an order
  CreateOrder requires a USER principal. A SERVICE principal gets permission_denied whatever its
  scopes and neither reserves stock nor creates an order: no service places orders on a buyer's
  behalf. The SERVICE token is signed with the local dev identity key and carries the buyer's id
  as its subject, so the buyer's own cart is the one it would check out.
  (port-edge-authz-residuals / order-checkout-correctness)

  Scenario: A service token cannot place an order
    Given a seller "sa" who owns a listing "L" with stock 5
    And a buyer "b1"
    And "b1" has 1 of "L" in the cart
    When a client calls CreateOrder through the gateway with a validly signed SERVICE token for the cart of "b1"
    Then the call fails with "permission_denied"
    And "b1" has 0 orders
    And the stock of "L" is unchanged at 5
