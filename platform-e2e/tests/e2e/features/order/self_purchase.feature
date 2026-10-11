@order @buyer @seller
Feature: Checkout reserves with least privilege and a seller cannot buy their own listing
  team-order reserves stock and redeems the voucher as its own service principal and
  refuses an order whose buyer is the seller of an item in it, before reserving anything.

  Scenario: A buyer checkout reserves and commits stock and voucher
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And an admin created a 10 percent voucher with a quota of 5
    And a commerce buyer with a saved address
    When the buyer checks out 2 of the listing with the voucher and pays with the mock payment
    Then the listing's stock has decreased by 2
    And the voucher redemption is committed once

  Scenario: A seller cannot check out their own listing
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And the seller has a saved address
    When the seller adds their own listing to their cart and places the order
    Then the order is refused with HTTP 400
    And the listing's stock is unchanged at 10
    And the seller has no orders
