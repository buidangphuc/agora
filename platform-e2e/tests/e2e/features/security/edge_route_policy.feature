Feature: The gateway edge enforces a route policy
  team-gateway refuses to route internal-only RPCs, gates admin-only RPCs on the
  admin scope before forwarding, reports upstream failures without internal detail,
  rejects tokens without exp or sub and validates the request-id and idempotency
  headers. Defence in depth in front of each service's own checks.

  Scenario: Internal stock and voucher saga RPCs answer 501 at the edge
    Given a logged-in seller with a listing in stock
    When the seller calls ReserveStock, ReleaseStock, CommitReservation and ReleaseReservation through the gateway
    Then each call answers HTTP 501 with code unimplemented
    And the listing's stock is unchanged

  Scenario: Writing an audit event is not exposed at the edge
    Given a logged-in seller
    And the seeded admin
    When the seller and the seeded admin call WriteAuditEvent through the gateway for a fresh target type
    Then the gateway answers HTTP 501 for each write
    And no audit event is stored for that target type

  Scenario: Anonymous call to an admin-only RPC
    Given an anonymous client
    When the client calls QueryAuditLog, ReviewKyc and ResolveDispute through the gateway
    Then each admin-only call answers HTTP 401

  Scenario: Buyer call to an admin-only RPC
    Given a logged-in buyer
    When the buyer calls QueryAuditLog, ReviewKyc and ResolveDispute through the gateway
    Then each admin-only call answers HTTP 403

  Scenario: Admin may query the audit log
    Given the seeded admin
    When the seeded admin calls QueryAuditLog through the gateway
    Then the request succeeds with an events list

  Scenario: A buyer cannot force-fail their own order
    Given a seller "sa" who owns a listing "L" with stock 10
    And a buyer "b1"
    And "b1" has a Pending order for 2 of "L"
    When the buyer "b1" calls ForceFailSaga on their own order through the gateway
    Then the gateway answers HTTP 403 to the force-fail call
    And the order read by "b1" remains Pending
    And the stock of "L" is unchanged at 8

  @destructive
  Scenario: An unreachable upstream is reported without internal detail
    Given the seeded admin
    And team-audit is stopped
    When the seeded admin calls QueryAuditLog through the gateway while team-audit is down
    Then the gateway answers with a fixed upstream-failure message
    And the body contains no host, port or dial error text

  Scenario: A signed token without an expiry is rejected
    Given a token signed by the identity key is accepted when it carries exp and sub
    When a client calls an authenticated RPC with a token signed by the identity key that has no exp claim
    Then the gateway answers HTTP 401

  Scenario: A signed token without a subject is rejected
    Given a token signed by the identity key is accepted when it carries exp and sub
    When a client calls an authenticated RPC with a token signed by the identity key whose sub is empty
    Then the gateway answers HTTP 401

  Scenario: A well-formed request id is echoed
    When a client sends X-Request-Id "e2e-req.42" on an RPC
    Then the response header X-Request-Id is "e2e-req.42"

  Scenario: A malformed request id is replaced
    When a client sends an X-Request-Id containing spaces and angle brackets
    Then the response header X-Request-Id is a different, well-formed id

  Scenario: A malformed idempotency key is rejected
    Given a logged-in buyer
    When the buyer calls CreateOrder with an Idempotency-Key longer than 255 bytes
    Then the gateway answers HTTP 400
