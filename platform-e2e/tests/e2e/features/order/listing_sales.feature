@order @integration
Feature: A listing's sold count is published as an absolute value
  team-order keeps a per-listing sold count (paid minus cancelled-from-PAID or refunded) in the
  same transaction as each order transition and publishes it on order.events as a
  ListingSalesChanged message keyed by listing_id (order-listing-sales). The count is observed
  black box through those messages: each message carries the absolute post-change value.

  Scenario: Payment adds the order quantity
    Given a seller with a listing "L"
    And a buyer "b1"
    When "b1" pays an order "O" of 2 of "L"
    Then the ListingSalesChanged values for "L" are exactly 2

  Scenario: A payment emits the new absolute value
    Given a seller with a listing "L"
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has paid an order "O1" of 3 of "L"
    When "b2" pays an order "O2" of 2 of "L"
    Then the ListingSalesChanged values for "L" are exactly 3, 5

  Scenario: Cancel from PAID subtracts the order quantity
    Given a seller with a listing "L"
    And a buyer "b1"
    And "b1" has paid an order "O1" of 3 of "L"
    And "b1" has paid an order "O2" of 2 of "L"
    When "b1" cancels the order "O2"
    Then the ListingSalesChanged values for "L" are exactly 3, 5, 3

  Scenario: Refunded return subtracts the order quantity
    Given a seller with a listing "L"
    And a buyer "b1"
    And "b1" has paid an order "O1" of 3 of "L"
    And "b1" has paid an order "O2" of 2 of "L"
    When "b1" requests a return of the order "O2" and the seller refunds it
    Then the ListingSalesChanged values for "L" are exactly 3, 5, 3

  Scenario: Cancel before PAID changes nothing
    Given a seller with a listing "L"
    And a buyer "b1"
    And a buyer "b2"
    And "b1" has a pending order "O1" of 2 of "L"
    When "b1" cancels the order "O1"
    And "b2" pays an order "O2" of 1 of "L"
    Then the ListingSalesChanged values for "L" are exactly 1

  Scenario: A repeated or redelivered transition changes nothing
    Given a seller with a listing "L"
    And a buyer "b1"
    And "b1" has paid an order "O" of 2 of "L"
    And "b1" cancels the order "O"
    When "b1" cancels the order "O" again
    Then the ListingSalesChanged values for "L" are exactly 2, 0

  Scenario: A cancel and a refund of the same order subtract once
    Given a seller with a listing "L"
    And a buyer "b1"
    And "b1" has paid an order "O1" of 3 of "L"
    And "b1" has paid an order "O2" of 2 of "L"
    And "b1" cancels the order "O2"
    When "b1" requests a return of the order "O2"
    Then the return request is refused
    And the ListingSalesChanged values for "L" are exactly 3, 5, 3

  Scenario: A multi-listing order emits one event per listing
    Given a seller with listings "L1" and "L2"
    And a buyer "b1"
    When "b1" pays one order "O" of 1 of "L1" and 4 of "L2"
    Then the ListingSalesChanged values for "L1" are exactly 1
    And the ListingSalesChanged values for "L2" are exactly 4

  Scenario: The payload carries no personal identifier
    Given a seller with a listing "L"
    And a buyer "b1"
    When "b1" pays an order "O" of 2 of "L"
    Then the ListingSalesChanged message for "L" is keyed by the listing id and holds only listing_id, sold_count and changed_at
    And the ListingSalesChanged record for "L" mentions neither the order, the buyer nor the seller

  Scenario: A client cannot set the sold count
    Given a seller with a listing "L"
    And a buyer "b1"
    When "b1" sends cart and order requests carrying a sold count of 999 for "L"
    And "b1" pays an order "O" of 2 of "L"
    Then the ListingSalesChanged values for "L" are exactly 2
