Feature: Streams get the same edge policy as unary calls
  team-gateway applies the unary edge policy to the StreamChat server stream: a presented
  but invalid token is refused instead of downgraded to anonymous, the request id is
  echoed, the per-caller rate limit is charged, and an oversized request is refused before
  it reaches the upstream. The stream is called with the Connect streaming envelope over
  HTTP. Change: port-edge-authz-residuals (edge-stream-and-http-policy).

  Scenario: An invalid token on a stream is refused
    Given a logged-in buyer for the stream checks
    When a client calls StreamChat through the gateway with a bearer token whose signature is invalid
    Then the stream ends with the Connect error code unauthenticated and no chat message is streamed

  Scenario: A stream echoes the request id
    Given a logged-in buyer for the stream checks
    When the buyer calls StreamChat through the gateway with a random X-Request-Id starting "e2e-stream-"
    Then the stream completes and the response carries that X-Request-Id

  Scenario: Streams are rate limited per caller
    Given a freshly registered buyer for the stream checks
    And a gateway whose rate-limit burst is small enough to exceed
    When the buyer opens more StreamChat calls at once than the rate-limit burst
    Then at least one of them fails with resource_exhausted
    And a different buyer's StreamChat call made right after succeeds

  Scenario: An oversized stream request is refused
    Given a logged-in buyer for the stream checks
    When the buyer calls StreamChat with a 20000-byte message
    Then the stream ends with the Connect error code resource_exhausted and no chat message is streamed
