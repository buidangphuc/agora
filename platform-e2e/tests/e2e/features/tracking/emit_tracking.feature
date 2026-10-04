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

  @needsBuyer @needsListing
  Scenario: Multi-item ecommerce beacons fan out and carry financial and group attributes
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the gateway receives a batched ecommerce payload with GA4 aliases
    Then multiple EventEnvelopes sharing the same event group id are published to the "analytics.events" topic
    And the purchase tracking payload carries transaction id, currency and minor unit prices

  @needsBuyer @needsListing
  Scenario: Impression and click beacons keep their placement attribution and the seller funnel counts them
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the gateway receives an impression and a click beacon with placement "home.recs" for the listing
    Then both beacons are published to the "analytics.events" topic carrying placement "home.recs"
    And the seller analytics funnel counts the impression

  # ui-phase-product-detail: the redesign must not add or drop view events.
  @needsSeller
  Scenario: A variant change and an anchor-nav click send no further view event from the browser
    Given a buyer is logged in
    And a seeded listing with variants "128GB" at 1000000 and "256GB" at 1500000
    When the buyer opens the variant listing while recording tracking beacons
    And the buyer selects the variant "256GB"
    And the buyer follows the anchor link "Đánh giá"
    Then exactly one view beacon for the listing was sent from the browser
