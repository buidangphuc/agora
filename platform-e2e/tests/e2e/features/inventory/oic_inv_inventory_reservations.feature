@order
Feature: Stock reservations have one lifecycle owned by team-domain
  A checkout holds the ordered quantity once, a placed order keeps it after the reservation
  TTL, and a failed checkout or a cancelled order returns it exactly once. Stock is observed
  through the gateway (GetListing) because the reservation RPCs are not routed at the edge.
  Scenarios that wait out the reservation TTL need the short-TTL overlay
  platform-e2e/compose/order-inventory.override.yaml (RESERVATION_TTL=20s,
  RESERVATION_SWEEP_INTERVAL=2s on team-domain and team-order); they run in the serial/overlay
  lane only.

  Scenario: A checkout holds the ordered quantity exactly once
    Given a seller with a listing whose stock is 10
    And a buyer with a saved address
    When the buyer checks out quantity 2 of the listing through the gateway
    Then the listing's stock read through the gateway is 8

  # overlay: platform-e2e/compose/order-inventory.override.yaml
  @destructive
  Scenario: A placed order keeps its stock after the reservation TTL
    Given the reservation sweep wait fits the run
    And a seller with a listing whose stock is 10
    And a buyer with a saved address
    When the buyer checks out quantity 2 of the listing through the gateway
    And more than the reservation TTL plus two sweep intervals of both services pass
    Then the listing's stock read through the gateway is 8
    And the buyer's order is still Pending

  # overlay: platform-e2e/compose/order-inventory.override.yaml
  @destructive
  Scenario: A failed checkout returns its stock once even after the TTL sweep
    Given the reservation sweep wait fits the run
    And two sellers whose listings have stock 10 and 1
    And a buyer with a saved address
    And the buyer's cart holds quantity 2 of the first listing and quantity 5 of the second
    When the buyer's checkout fails because the second seller's item is out of stock
    And more than the reservation TTL plus two sweep intervals of both services pass
    Then the first seller's listing stock read through the gateway equals its stock before the checkout

  # overlay: platform-e2e/compose/order-inventory.override.yaml
  @destructive
  Scenario: A cancelled order's stock is returned once even after the TTL sweep
    Given the reservation sweep wait fits the run
    And a seller with a listing whose stock is 10
    And a buyer with a saved address
    And the buyer has checked out quantity 2 of the listing
    When the buyer cancels the Pending order
    And more than the reservation TTL plus two sweep intervals of both services pass
    Then the listing's stock read through the gateway is 10

  Scenario: An invalid reservation TTL falls back to the default
    When the team-domain image is started with RESERVATION_TTL "banana" and RESERVATION_SWEEP_INTERVAL "0s"
    Then it starts and logs a warning naming each variable
    And it logs the effective TTL "15m0s" and interval "1m0s"

  # overlay: platform-e2e/compose/order-inventory.override.yaml
  @destructive
  Scenario: The e2e stack runs the sweeper on the configured short cadence
    Given the stack runs with the e2e reservation overlay
    Then team-domain and team-order each log the overlay values as their effective reservation TTL and sweep interval

  Scenario: A checkout announces the listing's new stock
    Given a seller with a listing whose stock is 10
    And a buyer with a saved address
    When the buyer checks out quantity 2 of the listing through the gateway
    Then a ListingStockChanged envelope keyed by that listing id with stock 8 appears on listing.events

  Scenario: A cancel announces the restored stock
    Given a seller with a listing whose stock is 10
    And a buyer with a saved address
    And the buyer has checked out quantity 2 of the listing
    When the buyer then cancels that order
    Then a later ListingStockChanged envelope for the listing with stock 10 appears on listing.events
