Feature: The gateway logs a keyed principal fingerprint, never the raw id
  Every edge.request line in the team-gateway container log carries principal_kind and, for an
  authenticated caller, a keyed principal_fp; the raw principal id never appears in the log.
  Change: gateway-principal-log-fingerprint (initiative p10-backend-gaps).

  Scenario: GW-FP-14 e2e-observable gateway log check
    Given a freshly registered user
    When the user calls an authenticated listing route with a fresh X-Request-Id
    Then the gateway log has an edge.request line for that request id
    And that line has principal_kind "user" and a 12 lowercase hex principal_fp
    And that line has no principal key
    And the gateway log since the scenario started does not contain the user's id

  Scenario: GW-FP-2 HTTP request by a user logs fingerprint and kind
    Given a freshly registered user
    When the user calls an authenticated listing route with a fresh X-Request-Id
    Then the gateway log has an edge.request line for that request id
    And that line has principal_kind "user" and a 12 lowercase hex principal_fp
    And that line has no principal key
    And the gateway log since the scenario started does not contain the user's id

  Scenario: GW-FP-4b anonymous HTTP caller has kind and no fingerprint
    Given a freshly registered user
    And the user calls an authenticated listing route with a fresh X-Request-Id
    When an anonymous caller calls a public listing route with a fresh X-Request-Id
    Then the gateway log has an edge.request line for that request id
    And that line has principal_kind "anonymous" and no principal_fp
    And that line has no principal key
    And the earlier user request line carried a principal_fp

  Scenario: GW-FP-5 invalid token is not logged as a user
    Given a freshly registered user
    And the user calls an authenticated listing route with a fresh X-Request-Id
    When a caller with an invalid bearer token calls a listing route with a fresh X-Request-Id
    Then the call is refused as unauthenticated
    And the gateway log has an edge.request line for that request id
    And that line has principal_kind "anonymous" and no principal_fp
    And the earlier user request line carried a principal_fp

  Scenario: GW-FP-6c fingerprint distinguishes ids
    Given a freshly registered user
    And a second freshly registered user
    When each user calls an authenticated listing route with a fresh X-Request-Id
    And the first user calls the route again with a fresh X-Request-Id
    Then both users' lines carry a 12 lowercase hex principal_fp
    And the two users' principal_fp values differ
    And the first user's two lines carry the same principal_fp

  Scenario: GW-FP-11 secret is never logged
    Given a freshly registered user
    When the user calls an authenticated listing route with a fresh X-Request-Id
    Then the gateway container has a non-empty LOG_KEY_FP_SECRET
    And that line has principal_kind "user" and a 12 lowercase hex principal_fp
    And the gateway log since the scenario started does not contain the LOG_KEY_FP_SECRET value
