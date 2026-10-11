Feature: The tracking collector is rate limited and observable
  team-gateway puts the plain-HTTP collector route POST /api/track behind the edge policy:
  a per-visitor beacon bucket answers 429 before anything is produced, and a validated
  X-Request-Id is echoed. Change: port-edge-authz-residuals (edge-stream-and-http-policy).

  # Destructive lane: the flood drains the anonymous per-IP beacon bucket, which every other
  # test's browser on the docker host shares.
  @destructive
  Scenario: A beacon flood is throttled
    Given the analytics.events topic is readable
    When one anonymous visitor sends 40 POST /api/track batches back to back, each with one event carrying a unique marker
    Then some requests answer HTTP 429
    And none of the markers of the 429 requests reach analytics.events

  Scenario: The collector echoes the request id
    When a visitor sends one valid POST /api/track batch with a random X-Request-Id starting "e2e-track-"
    Then the track response is accepted and carries that X-Request-Id
