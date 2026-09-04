@buyer @needsBuyer @needsListing @needsOrder
Feature: Saga failure triggers compensation
  As a buyer, if payment fails the saga compensates and releases stock.

  Scenario: Forced payment failure triggers saga compensation and releases stock
    Given a buyer has a pending order
    When the payment step is forced to fail
    Then stock is released and the order is CANCELLED
