@order
Feature: team-order commits stock as the order service
  The checkout's stock hold is committed by team-order's own service principal, not the buyer's scopes,
  so a placed order keeps its stock after the reservation TTL.
  (port-order-inventory-correctness / order-upstream-principals)

  # @destructive: waits the reservation TTL plus two sweep intervals, so it needs the short-TTL overlay
  # platform-e2e/compose/order-inventory.override.yaml (RESERVATION_TTL=20s, RESERVATION_SWEEP_INTERVAL=2s)
  # (without it the default 15m TTL would make the wait impractical). Serial lane only.
  @destructive
  Scenario: A buyer checkout commits stock as the order service
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has 2 of "L" in the cart
    When "b1" checks out
    And the reservation TTL plus two sweep intervals pass
    Then the order exists and is Pending
    And the stock of "L" is 8
