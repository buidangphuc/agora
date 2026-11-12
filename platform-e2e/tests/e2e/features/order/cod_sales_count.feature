@order @integration
Feature: A completed COD order raises the listing's sold count in search
  A COD order never reaches PAID, so team-order counts it into the per-listing sold count when it
  moves from SHIPPED to COMPLETED, whoever completes it, and publishes the absolute value as
  ListingSalesChanged (order-cod-sales-count). The count is observed black box as the sold_count
  of the listing in team-search. An online-paid order is counted at PAID only, and an order is
  counted once however often its completion is attempted.

  Scenario: A COD order completed by the buyer is counted
    Given a seller with a listing "L" indexed in search
    And a buyer "b1"
    And "b1" has a shipped COD order "O" of 2 of "L"
    When "b1" completes the order "O" by confirming receipt
    Then the search sold count of "L" rises by 2

  Scenario: A COD order completed by the seller is counted
    Given a seller with a listing "L" indexed in search
    And a buyer "b1"
    And "b1" has a shipped COD order "O" of 2 of "L"
    When the seller completes the order "O"
    Then the search sold count of "L" rises by 2

  Scenario: A COD order completed by the auto-complete sweep is counted
    Given a seller with a listing "L" indexed in search
    And a buyer "b1"
    And "b1" has a shipped COD order "O" of 2 of "L"
    When the auto-complete sweep completes the order "O"
    Then the search sold count of "L" rises by 2

  Scenario: Completing an online-paid order does not count it again
    Given a seller with a listing "L" indexed in search
    And a buyer "b1"
    And "b1" has paid an order "O" of 2 of "L"
    And the search sold count of "L" rises by 2
    And the seller ships the order "O"
    When "b1" completes the order "O" by confirming receipt
    Then the search sold count of "L" stays risen by 2

  Scenario: A repeated or concurrent COD completion counts once
    Given a seller with a listing "L" indexed in search
    And a buyer "b1"
    And "b1" has a shipped COD order "O" of 2 of "L"
    When "b1" completes the order "O" by confirming receipt
    And "b1" completes the order "O" by confirming receipt again
    Then the second completion is refused with a status conflict
    And the search sold count of "L" rises by 2
    And the search sold count of "L" stays risen by 2
