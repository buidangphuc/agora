@tracking @analytics
Feature: The consent-aware TrackingEvent contract stays compatible and the channel funnel is admin-only
  ga4-consent-contract adds ga_client_id (25) and consent (26) to TrackingEvent and an admin-only
  GetChannelFunnel RPC. A producer that does not set the new fields must keep flowing to the
  warehouse unchanged and read as consent denied; GetChannelFunnel is enforced by the serving
  service (tracking-consent). The channel funnel scenarios stay red until add-ga4-export-import
  serves the RPC.

  Scenario: Old payload stays readable
    When a visitor posts a view beacon with no consent cookie and no GA client id
    Then the warehouse holds one row for that listing carrying the beacon's session id and page path
    And the envelope on analytics.events has no GA client id

  Scenario: Missing consent reads as denied
    When a visitor posts a view beacon with no consent cookie and no GA client id
    Then the envelope on analytics.events grants no consent, with analytics, marketing and personalization false and version 0

  Scenario: Non-admin is rejected
    Given a signed-in seller
    When the seller requests GetChannelFunnel for their own seller id
    Then the channel funnel call returns 403 and no rows
    When a signed-in buyer requests GetChannelFunnel with no seller id
    Then the channel funnel call returns 403 and no rows

  Scenario: Empty seller means all sellers
    Given an admin is logged in
    When the admin requests GetChannelFunnel for the last 30 days with no seller id
    And the admin requests GetChannelFunnel for the last 30 days for a seller with no events
    Then both channel funnel calls return 200
    And the seller-scoped funnel has no rows
    And no channel funnel count of the seller-scoped call exceeds the all-sellers total

  Scenario: Invalid date range
    Given an admin is logged in
    When the admin requests GetChannelFunnel with from after to
    Then the channel funnel call returns 400

  Scenario: Empty range returns no rows
    Given an admin is logged in
    When the admin requests GetChannelFunnel over a range that holds no events
    Then the channel funnel call returns 200 with zero rows
