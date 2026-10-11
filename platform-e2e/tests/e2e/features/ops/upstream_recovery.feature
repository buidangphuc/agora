Feature: The edge recovers quickly after an upstream restart
  After team-payment is recreated with a new address, team-gateway (not restarted) routes
  calls to it again within seconds. Change: port-edge-authz-residuals (edge-stream-and-http-policy).

  # Recreates agora-team-payment through the stack's compose wrapper; teardown waits until
  # the wallet read is stable again.
  @destructive
  Scenario: A recreated upstream is reachable again within seconds
    Given a logged-in seller whose wallet balance can be read through the gateway
    When the team-payment container is recreated and its gRPC port answers
    Then within 5 seconds the seller's GetWalletBalance through the gateway succeeds
    And it keeps succeeding on 3 consecutive calls
