Feature: The listing stock commit RPC is not exposed at the edge
  ListingService/CommitReservation is a service-to-service RPC (team-order -> team-domain).
  team-gateway never routes it, for any caller, so a buyer's reservation cannot be committed
  or tampered with from outside.

  Scenario: The listing stock commit RPC answers 501 at the edge
    Given a buyer's live order for quantity 2 of a seller's listing with stock 10
    When a logged-in seller, then an anonymous caller, calls ListingService/CommitReservation through the gateway with the reservation id of that order
    Then each commit call answers HTTP 501 with code unimplemented
    And the listing's stock is still the 8 left by the checkout
