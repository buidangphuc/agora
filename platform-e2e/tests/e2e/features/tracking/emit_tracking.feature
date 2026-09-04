@tracking @buyer
Feature: Browsing actions emit tracking events
  As the platform, a browsing action (view / click / add-to-cart / impression)
  is collected at the gateway edge and published as a TrackingEvent envelope on
  the analytics.events Kafka topic — best-effort, never blocking the browser.

  @needsBuyer @needsListing
  Scenario: A browsing beacon becomes a tracking event on analytics.events
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the gateway receives a valid track beacon for a product view
    Then exactly one EventEnvelope of type "platform.analytics.v1.TrackingEvent" is published to the "analytics.events" topic
    And its payload is a TrackingEvent with EventType "EVENT_TYPE_VIEW" carrying the listing id, session id and page path

  Scenario: A malformed beacon is rejected without producing
    When the gateway receives a beacon with no recognizable event type
    Then the gateway responds with a client error
    And nothing is published to the "analytics.events" topic

  @needsBuyer @needsListing
  Scenario: Authenticated browsing attributes the event via the envelope
    Given a buyer is logged in
    And a listing has been seeded via the API
    When a logged-in buyer's view beacon is collected at the gateway
    Then the produced EventEnvelope principal identifies the buyer
    And the TrackingEvent payload contains no authenticated user id in its own fields

  @needsBuyer @needsListing
  Scenario: A browsing action still succeeds when the event cannot be delivered
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer performs a tracked browsing action while the analytics producer is unavailable
    Then the browsing action completes normally
    And no user-visible error is shown
